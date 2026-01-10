package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/joho/godotenv"
	// Assuming these imports are correct for your project structure
	"github.com/nexora/nexora_segmentation/internal/api"
	// "github.com/nexora/nexora_segmentation/internal/db"
)

func main() {
	// 1. Load the .env file
	// Loads key/value pairs into the environment.
	if err := godotenv.Load(); err != nil {
		// Print a warning but don't panic, as variables might be set directly in the environment
		fmt.Println("Warning: Could not load .env file. Falling back to environment variables or defaults.")
	}

	// db.InitClickhouse()
	// db.InitMySQL()

	// 2. Get the port from the environment variable named "PORT"
	portStr := os.Getenv("PORT")

	// 3. Set a default port if the environment variable is not set
	if portStr == "" {
		portStr = "8080" // Default port
	}

	// 4. Construct the listen address string, which must be in the format ":<port>"
	listenAddr := fmt.Sprintf(":%s", portStr)

	// Set up the API routes
	http.HandleFunc("/health", api.HealthCheckHandler)
	http.HandleFunc("/segments/evaluate", api.EvaluateHandler)

	// Start the server
	fmt.Printf("🚀 Segmentation service running on %s\n", listenAddr)
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		panic(err)
	}
}
