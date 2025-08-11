package main

import (
	"fmt"
	"livescribble/internal/auth"
	"livescribble/internal/database"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	logFile, err := os.OpenFile("app.error_logger", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("Failed to open error_logger file")
	}
	defer func(logFile *os.File) {
		err := logFile.Close()
		if err != nil {
			log.Fatalln("Failed to close error_logger file")
		}
	}(logFile)

	error_logger := slog.New(slog.NewTextHandler(logFile, nil))

	db := database.NewDatabaseManager()
	err = db.Connect()
	if err != nil {
		error_logger.Error(fmt.Sprintf("error connecting to database: %v", err.Error()))
		return
	}

	defer func(db *database.Manager) {
		err := db.Close()
		if err != nil {
			error_logger.Error(fmt.Sprintf("error closing database: %v", err.Error()))
			log.Fatal(fmt.Sprintf("error closing database: %v", err.Error()))
			return
		}
	}(db)

	jwt := os.Getenv("JWT_KEY")
	if jwt == "" {
		error_logger.Error(fmt.Sprintf("No JWT_KEY found in environment"))
		return
	}

	r := gin.Default()

	authHandler := auth.NewHandler(db.DB, []byte(jwt), error_logger)

	r.POST("/login", authHandler.Login)
	r.POST("/register", authHandler.Register)
	r.GET("/health", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{})
	})
	protected := r.Group("/protected")
	protected.Use(auth.MiddleWare([]byte(jwt), db.DB))

	err = r.Run(":8081")
	if err != nil {
		error_logger.Error(fmt.Sprintf("error starting server: %v", err.Error()))
		log.Fatal(fmt.Sprintf("error starting server: %v", err.Error()))
	}

}
