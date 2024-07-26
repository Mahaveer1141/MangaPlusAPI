package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/imagekit-developer/imagekit-go"
	"github.com/imagekit-developer/imagekit-go/api/uploader"
)

func ConnectToMongoDB() (*mongo.Client, error) {
	// Use the SetServerAPIOptions() method to set the version of the Stable API on the client
	serverAPI := options.ServerAPI(options.ServerAPIVersion1)
	opts := options.Client().ApplyURI(os.Getenv("MONGO_URI")).SetServerAPIOptions(serverAPI)
	// Create a new client and connect to the server
	client, err := mongo.Connect(context.TODO(), opts)
	if err != nil {
		return nil, err
	}
	// Send a ping to confirm a successful connection
	return client, nil
}

func IntializeImageKit() (*imagekit.ImageKit, error) {
	// Using environment variables IMAGEKIT_PRIVATE_KEY, IMAGEKIT_PUBLIC_KEY and IMAGEKIT_ENDPOINT_URL
	ik, err := imagekit.New()
	if err != nil {
		return nil, err
	}
	return ik, nil
}

func UploadFile(c *gin.Context, file *multipart.FileHeader, filename string, ik *imagekit.ImageKit) (*uploader.UploadResponse, error) {
	src, err := file.Open()
	fileBytes, err := io.ReadAll(src)
	base64String := base64.StdEncoding.EncodeToString(fileBytes)
	resp, err := ik.Uploader.Upload(c, base64String, uploader.UploadParam{FileName: filename, Folder: "/MangaPlus"})
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func MongoCollection(client *mongo.Client, collectionName string) *mongo.Collection {
	return client.Database("mangaplus_dev").Collection(collectionName)
}

func Manga(client *mongo.Client) *mongo.Collection {
	return MongoCollection(client, "mangas")
}

type MangaData struct {
	url    string
	height int
	width  int
}

func main() {
	err := godotenv.Load()
	ik, err := IntializeImageKit()
	client, err := ConnectToMongoDB()
	if err != nil {
		panic(err)
	}
	router := gin.Default()

	router.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "You reached the server",
			"errors":  err,
		})
	})

	router.GET("/manga/:slug/volumes/:volume_number/chapters/:chapter_number", func(c *gin.Context) {
		chapter_number := c.Param("chapter_number")
		volume_number := c.Param("volume_number")
		query := bson.D{{"chapter_number", chapter_number}, {"volume_number", volume_number}}
		var chapter bson.M
		err = Manga(client).FindOne(context.TODO(), query).Decode(&chapter)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"chapter": chapter})
	})

	router.POST("/upload", func(c *gin.Context) {
		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"message": "Error parsing form",
			})
			return
		}
		files := form.File["files"]
		chapter_number := form.Value["chapter_number"][0]
		volume_number := form.Value["volume_number"][0]
		var responses = []bson.D{}
		for index, file := range files {
			response, err := UploadFile(c, file, fmt.Sprint(index, ".jpg"), ik)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"message": "Error uploading file",
					"error":   err.Error(),
				})
				return
			}
			manga_data := bson.D{{"url", response.Data.Url}, {"height", response.Data.Height}, {"width", response.Data.Width}}
			responses = append(responses, manga_data)
		}
		data := bson.D{{"data", responses}, {"volume_number", volume_number}, {"chapter_number", chapter_number}}
		result, err := Manga(client).InsertOne(context.TODO(), data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "success", "responses": result})
	})

	var port string = os.Getenv("PORT")
	router.Run(":" + port)
}
