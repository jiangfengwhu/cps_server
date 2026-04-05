package main

import (
	"cps-go/config"
	"cps-go/db"
	"cps-go/handler"
	"cps-go/platform/jd"
	"cps-go/platform/pdd"
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

	cfg, err := config.Load("config.json")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("failed to validate config: %v", err)
	}

	db.Connect(cfg.Database.URL)
	defer db.Close()

	jdClient, err := jd.NewClient(cfg.JDAppKey, cfg.JDSecretKey, cfg.JDUnionID)
	if err != nil {
		log.Fatalf("failed to create JD client: %v", err)
	}
	jdHandler := handler.NewJDHandler(jdClient)

	pddClient := pdd.NewClient(cfg.PDDClientId, cfg.PDDClientSecret, cfg.PDDPid)
	pddHandler := handler.NewPDDHandler(pddClient)

	gin.DefaultWriter = io.MultiWriter(os.Stdout, lj)
	gin.DefaultErrorWriter = io.MultiWriter(os.Stderr, lj)
	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	api := r.Group("/api")
	{
		jdGroup := api.Group("/jd")
		{
			jdGroup.GET("/test/goods", jdHandler.TestGoodsInfo)
			jdGroup.POST("/convert", jdHandler.ConvertLink)
			jdGroup.POST("/orders", jdHandler.QueryOrders)
		}

		pddGroup := api.Group("/pdd")
		{
			// pddGroup.GET("/authority/check", pddHandler.CheckAuthority)
			// pddGroup.GET("/authority/generate", pddHandler.GenerateAuthorityURL)
			// pddGroup.GET("/test/promote", pddHandler.TestPromotionURL)
			pddGroup.POST("/convert", pddHandler.ConvertLink)
			pddGroup.POST("/promote", pddHandler.Promote)
			pddGroup.POST("/search", pddHandler.SearchGoods)
			pddGroup.POST("/orders", pddHandler.QueryOrders)
		}
	}

	log.Printf("Server starting on port %s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
