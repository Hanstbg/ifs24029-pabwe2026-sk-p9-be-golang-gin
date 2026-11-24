package server

import (
	"net/http"
	"time"

	"app/docs"
	"app/internal/config"
	"app/internal/features/ai"
	"app/internal/features/auth"
	"app/internal/features/post"
	"app/internal/middleware"
	"app/internal/response"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"
)

func requestScheme(c *gin.Context) string {
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		return "https"
	}
	return "http"
}

// NewRouter merakit seluruh route API, Swagger UI, dan handler 404.
func NewRouter(cfg *config.Config, db *gorm.DB) *gin.Engine {
	r := gin.Default()
	r.Use(middleware.CORS())

	httpClient := &http.Client{Timeout: 90 * time.Second}
	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(db), cfg.AppKey))
	postHandler := post.NewHandler(post.NewService(post.NewRepository(db)))
	aiHandler := ai.NewHandler(ai.NewService(
		ai.NewSearchClient(cfg.SearchEngineEndpoint, httpClient),
		ai.NewChatClient(cfg.OpenAIEndpoint, cfg.OpenAIKey, cfg.OpenAIModel, httpClient),
	))

	r.GET("/api.json", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json; charset=utf-8",
			[]byte(docs.GetSwaggerJSON(c.Request.Host, requestScheme(c))))
	})
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/api.json")))

	api := r.Group("/api", middleware.RateLimiter())
	authRequired := middleware.Auth(cfg.AppKey)

	public := api.Group("/public")
	public.GET("/categories", postHandler.Categories)
	public.GET("/posts", postHandler.PublicList)
	public.GET("/posts/:slug", postHandler.PublicDetail)

	authGroup := api.Group("/auth")
	authGroup.POST("/register", authHandler.Register)
	authGroup.POST("/login", authHandler.Login)
	authGroup.POST("/logout", authRequired, authHandler.Logout)
	authGroup.GET("/profile", authRequired, authHandler.Profile)
	authGroup.PUT("/profile", authRequired, authHandler.UpdateProfile)

	posts := api.Group("/posts", authRequired)
	posts.GET("", postHandler.List)
	posts.POST("", postHandler.Create)
	posts.GET("/stats", postHandler.Stats)
	posts.POST("/ai-generate", aiHandler.Generate)
	posts.GET("/:id", postHandler.Get)
	posts.PUT("/:id", postHandler.Update)
	posts.PATCH("/:id", postHandler.Update)
	posts.PATCH("/:id/toggle-publish", postHandler.TogglePublish)
	posts.DELETE("/:id", postHandler.Delete)

	admin := api.Group("/admin", authRequired, middleware.RequireRole("admin"))
	admin.GET("/users", postHandler.AdminUsers)
	admin.GET("/posts", postHandler.AdminPosts)
	admin.GET("/users/:id/posts", postHandler.AdminUserPosts)

	r.NoRoute(func(c *gin.Context) {
		response.Fail(c, http.StatusNotFound, "Endpoint not found")
	})
	return r
}
