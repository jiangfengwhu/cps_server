package main

import (
	"cps-go/config"
	"cps-go/db"
	"cps-go/util"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	lj := util.InitLogger("logs")
	defer lj.Close()

	config, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	// validate config
	if err := config.Validate(); err != nil {
		log.Fatalf("failed to validate config: %v", err)
	}
	// connect to MongoDB
	db.Connect(config.Database.URL)
	defer db.Close()

	gin.DefaultWriter = io.MultiWriter(os.Stdout, lj)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, lj)
	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		// Return JSON response
		c.JSON(http.StatusOK, gin.H{
			"message": "pong",
		})
	})

	// Start server on port 8080 (default)
	// Server will listen on 0.0.0.0:8080 (localhost:8080 on Windows)
	if err := r.Run(":" + config.Port); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
