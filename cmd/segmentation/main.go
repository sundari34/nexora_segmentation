package main

import (
	"fmt"
	"net/http"

	"github.com/nexora/nexora_segmentation/internal/api"
	"github.com/nexora/nexora_segmentation/internal/db"
)

func main() {
	db.InitClickhouse()
	db.InitMySQL()

	http.HandleFunc("/health", api.HealthCheckHandler)
	http.HandleFunc("/segments/evaluate", api.EvaluateHandler)

	port := 8080
	fmt.Printf("🚀 Segmentation service running on :%d\n", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		panic(err)
	}
}
