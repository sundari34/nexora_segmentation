package main

import (
	"fmt"
	"net/http"
	"os"      // Import the 'os' package to read environment variables
	"strconv" // Import 'strconv' to convert the string port to an integer

	"github.com/joho/godotenv" // Import godotenv
	"github.com/nexora/nexora_segmentation/internal/api"
	"github.com/nexora/nexora_segmentation/internal/db"
)

func main() {
	// 1. Load the .env file
	// godotenv.Load() will look for a .env file in the current directory.
	// It loads the key/value pairs into the environment.
	if err := godotenv.Load(); err != nil {
		// Log the error but don't panic if the .env file isn't found,
		// as variables might be set directly in the environment.
		fmt.Println("Warning: Could not load .env file. Falling back to environment variables or defaults.")
	}

	db.InitClickhouse()
	db.InitMySQL()

	// 2. Get the port from the environment variable
	// os.Getenv("PORT") reads the value of the environment variable named "PORT".
	portStr := os.Getenv("PORT")

	// 3. Set a default port if the environment variable is not set
	if portStr == "" {
		portStr = "8080" // Default port
	}

	// 4. Convert the port string to an integer (optional, but good practice for internal use)
	port, err := strconv.Atoi(portStr)
	if err != nil {
		// Handle the case where the PORT variable is set but isn't a valid integer
		fmt.Printf("Error: Invalid port value in environment: %s. Defaulting to 8080.\n", portStr)
		port = 8080
	}

	// The port string is what you should use for http.ListenAndServe
	// because it expects a string in the format ":<port>".
	listenAddr := fmt.Sprintf(":%s", portStr)

	http.HandleFunc("/health", api.HealthCheckHandler)
	http.HandleFunc("/segments/evaluate", api.EvaluateHandler)

	fmt.Printf("🚀 Segmentation service running on %s\n", listenAddr)

	// Use the string format for http.ListenAndServe
	if err := http.ListenAndServe(listenAddr, nil); err != nil {
		panic(err)
	}
}
