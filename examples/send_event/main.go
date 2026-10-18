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

	// Send a webhook event to all subscribed endpoints
	result, err := client.Messages.Send(ctx, "app_YOUR_APP_ID", &hookbase.SendMessageParams{
		EventType: "order.created",
		Payload: map[string]interface{}{
			"orderId":    "ord_123",
			"customerId": "cust_456",
			"amount":     9999,
			"currency":   "usd",
		},
	})
	if err != nil {
		log.Fatalf("Failed to send event: %v", err)
	}

	fmt.Printf("Event sent: %s\n", result.EventID)
	fmt.Printf("Queued for %d endpoints\n", result.MessagesQueued)
	for _, ep := range result.Endpoints {
		fmt.Printf("  - Endpoint %s (%s)\n", ep.ID, ep.URL)
	}
}
