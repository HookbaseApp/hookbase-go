package main

import (
	"context"
	"fmt"
	"log"
	"os"

	hookbase "github.com/hookbase/hookbase-go"
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

	fmt.Printf("Message sent: %s\n", result.MessageID)
	fmt.Printf("Delivered to %d endpoints\n", len(result.OutboundMessages))
	for _, msg := range result.OutboundMessages {
		fmt.Printf("  - Endpoint %s: %s\n", msg.EndpointID, msg.Status)
	}
}
