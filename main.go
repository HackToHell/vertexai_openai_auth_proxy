package main

import (
	"context"
	"fmt"
	"golang.org/x/oauth2"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/oauth2/google"
)

type Config struct {
	ProjectID  string
	Location   string
	EndpointID string
}

type Server struct {
	config     Config
	credential *google.Credentials
	router     *gin.Engine
}

func NewServer(config Config) *Server {
	return &Server{
		config: config,
		router: gin.Default(),
	}
}

type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelList struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

// Define available models as a package-level constant
var AvailableModels = map[string]ModelObject{
	"google/gemini-2.0-flash-001": {
		ID:      "google/gemini-2.0-flash-001",
		Object:  "model",
		Created: 1706745600, // February 1, 2024
		OwnedBy: "google",
	},
}

// Simplified handlers using the single source of truth
func (s *Server) handleListModels(c *gin.Context) {
	modelList := ModelList{
		Object: "list",
		Data:   make([]ModelObject, 0, len(AvailableModels)),
	}

	for _, model := range AvailableModels {
		modelList.Data = append(modelList.Data, model)
	}

	c.JSON(http.StatusOK, modelList)
}

func (s *Server) handleRetrieveModel(c *gin.Context) {
	modelID := c.Param("model")

	model, exists := AvailableModels[modelID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "The model '" + modelID + "' does not exist",
				"type":    "invalid_request_error",
				"param":   nil,
				"code":    "model_not_found",
			},
		})
		return
	}

	c.JSON(http.StatusOK, model)
}

func (s *Server) setupRoutes() {
	s.router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"*"},
		ExposeHeaders:    []string{"*"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	s.router.POST("/v1/chat/completions", s.handleChatCompletions)
	s.router.OPTIONS("/v1/chat/completions", s.handleOptions)
	s.router.GET("/v1/models", s.handleListModels)
	s.router.GET("/v1/models/:model", s.handleRetrieveModel)
	// Add more OpenAI-compatible endpoints as needed
}

func (s *Server) refreshCredentials(ctx context.Context) error {
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return fmt.Errorf("failed to get credentials: %v", err)
	}

	token, err := creds.TokenSource.Token()
	if err != nil {
		return fmt.Errorf("failed to get token: %v", err)
	}

	s.credential = creds

	// Schedule next refresh before token expires
	go func() {
		time.Sleep(time.Until(token.Expiry.Add(-5 * time.Minute)))
		if err := s.refreshCredentials(ctx); err != nil {
			log.Printf("Failed to refresh credentials: %v", err)
		}
	}()

	return nil
}

func (s *Server) handleOptions(c *gin.Context) {
	c.Status(http.StatusOK)
}

func (s *Server) handleChatCompletions(c *gin.Context) {
	var request openai.ChatCompletionRequest
	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	clientConfig := openai.ClientConfig{}
	clientConfig.BaseURL = fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1beta1/projects/%s/locations/%s/endpoints/%s",
		s.config.Location, s.config.ProjectID, s.config.Location, s.config.EndpointID)
	httpClient := oauth2.NewClient(c.Request.Context(), s.credential.TokenSource)

	// Update the client configuration to use Google credentials
	clientConfig.HTTPClient = httpClient
	client := openai.NewClientWithConfig(clientConfig)

	request.Model = "google/gemini-2.0-flash-001"

	resp, err := client.CreateChatCompletion(c.Request.Context(), request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

type tokenTransport struct {
	token func() (*oauth2.Token, error)
}

func (t *tokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	token, err := t.token()
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	return http.DefaultTransport.RoundTrip(req)
}

func main() {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	config := Config{
		ProjectID:  projectID,
		Location:   "us-central1",
		EndpointID: "openapi", // or your specific endpoint ID
	}

	server := NewServer(config)

	ctx := context.Background()
	if err := server.refreshCredentials(ctx); err != nil {
		log.Fatalf("Failed to initialize credentials: %v", err)
	}

	server.setupRoutes()

	if err := server.router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

//https://us-central1-aiplatform.googleapis.com/v1beta1/projects/ca-observability-gemi-dev-i8pt/locations/us-central1/endpoints/openapi
