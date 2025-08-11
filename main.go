package main

import (
	"context"
	"errors"
	"fmt"
	"livescribble/internal/auth"
	"livescribble/internal/database"
	"livescribble/internal/utils"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func main() {
	logFile, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("Failed to open log file")
	}
	defer func(logFile *os.File) {
		err := logFile.Close()
		if err != nil {
			log.Fatalln("Failed to close errorLogger file")
		}
	}(logFile)

	errorLogger := slog.New(slog.NewTextHandler(logFile, nil))

	db := database.NewDatabaseManager()
	err = db.Connect()
	if err != nil {
		errorLogger.Error(fmt.Sprintf("error connecting to database: %v", err.Error()))
		return
	}

	defer func(db *database.Manager) {
		err := db.Close()
		if err != nil {
			errorLogger.Error(fmt.Sprintf("error closing database: %v", err.Error()))
			log.Fatal(fmt.Sprintf("error closing database: %v", err.Error()))
			return
		}
	}(db)

	var ctxt = context.Background()
	redisClient, err := utils.NewRedisClient()
	if err != nil {
		errorLogger.Error(fmt.Sprintf("error connecting to redis: %v", err.Error()))
		log.Fatal(fmt.Sprintf("error connecting to redis: %v", err.Error()))
	}
	if _, err := redisClient.Ping(ctxt).Result(); err != nil {
		errorLogger.Error(fmt.Sprintf("error pinging redis: %v", err.Error()))
		log.Fatal(fmt.Sprintf("error pinging redis: %v", err.Error()))
	}

	jwt := os.Getenv("JWT_KEY")
	if jwt == "" {
		errorLogger.Error(fmt.Sprintf("No JWT_KEY found in environment"))
		return
	}

	r := gin.Default()

	authHandler := auth.NewHandler(db.DB, []byte(jwt), errorLogger)

	r.POST("/login", authHandler.Login)
	r.POST("/register", authHandler.Register)
	r.GET("/health", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{})
	})
	protected := r.Group("/protected")
	protected.Use(auth.MiddleWare([]byte(jwt), db.DB))
	{
		protected.GET("/document", func(ctx *gin.Context) {
			requestedDocId := ctx.Param("doc_id")
			currentUser := ctx.GetString
			var document utils.Document
			err := db.DB.Where("id = ? AND user_id =? ", requestedDocId, currentUser).First(&document).Error
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					ctx.JSON(http.StatusNotFound, gin.H{
						"message": "document not found",
					})
				} else {
					ctx.JSON(http.StatusInternalServerError, gin.H{
						"message": "error retrieving document",
					})
				}
				return
			}
			ctx.JSON(http.StatusOK, gin.H{
				"document": document,
			})
		})
	}

	err = r.Run(":8081")
	if err != nil {
		errorLogger.Error(fmt.Sprintf("error starting server: %v", err.Error()))
		log.Fatal(fmt.Sprintf("error starting server: %v", err.Error()))
	}

}
