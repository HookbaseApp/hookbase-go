package main

import (
	"context"
	"fmt"
	"log"
	"os"

	hookbase "github.com/HookbaseApp/hookbase-go"
)

func main() {
	apiKey := os.Getenv("HOOKBASE_API_KEY")
	if apiKey == "" {
		log.Fatal("HOOKBASE_API_KEY environment variable is required")
	}

	client := hookbase.New(apiKey)
	ctx := context.Background()

	// List all inbound webhook sources
	page, err := client.Sources.List(ctx, &hookbase.ListSourcesParams{
		PageSize: hookbase.Ptr(10),
	})
	if err != nil {
		log.Fatalf("Failed to list sources: %v", err)
	}

	fmt.Printf("Found %d sources (page %d):\n", page.Total, page.Page)
	for _, source := range page.Data {
		status := "active"
		if !source.IsActive {
			status = "inactive"
		}
		fmt.Printf("  [%s] %s (%s) - %s - %d events\n",
			source.ID, source.Name, source.Provider, status, source.EventCount)
	}

	if page.HasMore {
		fmt.Println("\n  (more sources available...)")
	}
}
