package main

import (
	"context"
	"errors"
	"fmt"
	"livescribble/internal/auth"
	"livescribble/internal/database"
	"livescribble/internal/room"
	"livescribble/internal/utils"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

func main() {
	// Setup Logging
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

	// WebSocket upgrader
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true //TODO:restrict
		},
	}
	// 	Get JWT
	jwt := os.Getenv("JWT_KEY")
	if jwt == "" {
		errorLogger.Error("No JWT_KEY found in environment")
		return
	}
	// connect to Postgres
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
			log.Fatalf("%s", fmt.Sprintf("error closing database: %v", err.Error()))
			return
		}
	}(db)

	var ctxt = context.Background()
	// connect to Redis
	redisClient, err := utils.NewRedisClient()
	if err != nil {
		errorLogger.Error(fmt.Sprintf("error connecting to redis: %v", err.Error()))
		log.Fatalf("%s", fmt.Sprintf("error connecting to redis: %v", err.Error()))
	}
	if _, err := redisClient.Ping(ctxt).Result(); err != nil {
		errorLogger.Error(fmt.Sprintf("error pinging redis: %v", err.Error()))
		log.Fatalf("%s", fmt.Sprintf("error pinging redis: %v", err.Error()))
	}

	r := gin.Default()

	authHandler := auth.NewHandler(db.DB, []byte(jwt), errorLogger)

	// Initialize room manager
	roomManager := room.NewRoomManager(db.DB, errorLogger, redisClient)

	r.POST("/login", authHandler.Login)
	r.POST("/register", authHandler.Register)
	r.GET("/health", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{})
	})
	protected := r.Group("/protected")
	protected.Use(auth.MiddleWare([]byte(jwt), db.DB))
	{
		protected.GET("/document/:doc_id", func(ctx *gin.Context) {
			requestedDocId := ctx.Param("doc_id")
			currentUser := ctx.GetString("current_user")
			var document utils.Document
			err := db.DB.Where("id = ?", requestedDocId).First(&document).Error
			if err != nil || (document.UserID != currentUser && !containsKeyword(document.Access, currentUser)) {
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

		// Create a new document
		protected.POST("/document", func(ctx *gin.Context) {
			currentUser := ctx.GetString("current_user")

			new_document_id, err := generateDocumentId()
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{
					"message": "error creating document",
				})
				return
			}
			document := utils.Document{
				ID:      new_document_id,
				UserID:  currentUser,
				Content: "",
				Access:  []string{},
			}

			err = db.DB.Create(&document).Error
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{
					"message": "error creating document",
				})
				return
			}

			ctx.JSON(http.StatusCreated, gin.H{
				"document": document,
			})
		})

		protected.GET("/ws/:doc_id", func(ctx *gin.Context) {
			docId := ctx.Param("doc_id")
			currentUser := ctx.GetString("current_user")

			if len(docId) == 0 || len(docId) > 50 {
				ctx.JSON(http.StatusBadRequest, gin.H{
					"message": "invalid document ID",
				})
				return
			}
			// Verify user has access to the document
			var document utils.Document
			err := db.DB.Where("id = ?", docId).First(&document).Error
			if err != nil || (document.UserID != currentUser && !containsKeyword(document.Access, currentUser)) {
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

			// Upgrade HTTP connection to WebSocket
			conn, err := upgrader.Upgrade(ctx.Writer, ctx.Request, nil)
			if err != nil {
				errorLogger.Error("Failed to upgrade connection", "error", err, "docId", docId, "userId", currentUser)
				return
			}

			roomManager.JoinRoom(docId, conn)
		})
	}

	err = r.Run(":8081")
	if err != nil {
		errorLogger.Error(fmt.Sprintf("error starting server: %v", err.Error()))
		log.Fatalf("%s", fmt.Sprintf("error starting server: %v", err.Error()))
	}

}

func generateDocumentId() (string, error) {
	return utils.RandomString(10)
}
func containsKeyword(list []string, keyword string) bool {
	for _, item := range list {
		if strings.Contains(item, keyword) {
			return true
		}
	}
	return false
}
