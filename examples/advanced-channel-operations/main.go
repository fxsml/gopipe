package main

import (
	"context"
	"fmt"

	"github.com/fxsml/gopipe/channel"
)

type Article struct {
	ID   string
	Name string
	Shop string
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create input with two single articles
	ch1 := channel.FromSlice([]Article{
		{ID: "CH1.1", Name: "Laptop"},
		{ID: "CH1.2", Name: "Phone"},
	})

	// Create input with one slice of articles
	ch2 := channel.FromValues([]Article{
		{ID: "CH3.1", Name: "Tablet"},
		{ID: "CH3.2", Name: "Watch"},
		{ID: "CH3.3", Name: "Sensor"},
	})

	// Merge article channels and flatten slices from ch2
	articlesCh := channel.Merge(ch1, channel.Flatten(ch2))

	// Create a list of shops
	shops := []string{"ShopA", "ShopB"}

	// Add cancellation handling before further processing
	// to stop processing on context cancellation
	articlesCh = channel.Cancel(ctx, articlesCh, func(a Article, err error) {
		fmt.Printf("Processing article %s canceled: %v\n", a.ID, err)
	})

	// Expand articles to multiple shops
	articlesCh = channel.Process(articlesCh, func(a Article) []Article {
		articles := make([]Article, len(shops))
		for i, shop := range shops {
			articles[i] = Article{
				ID:   a.ID,
				Name: a.Name,
				Shop: shop,
			}
		}
		return articles
	})

	// Route shop articles based on shop name
	routed := channel.Route(articlesCh, func(a Article) int {
		switch a.Shop {
		case "ShopA":
			return 0
		case "ShopB":
			return 1
		default:
			return -1
		}
	}, len(shops))

	// Create sinks for each shop
	doneChans := make([]<-chan struct{}, len(shops))
	for i, r := range routed {
		doneChans[i] = channel.Sink(r, func(a Article) {
			fmt.Printf("%s: %s (%s)\n", a.Shop, a.Name, a.ID)
		})
	}

	// Wait for all sinks to complete
	<-channel.Merge(doneChans...)

	// Output:
	// ShopA: Laptop (CH1.1)
	// ShopA: Tablet (CH3.1)
	// ShopB: Laptop (CH1.1)
	// ShopB: Tablet (CH3.1)
	// ShopA: Phone (CH1.2)
	// ShopB: Phone (CH1.2)
	// ShopA: Watch (CH3.2)
	// ShopA: Sensor (CH3.3)
	// ShopB: Watch (CH3.2)
	// ShopB: Sensor (CH3.3)
}
