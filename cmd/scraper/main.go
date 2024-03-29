package main

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

type ScrapeResult struct {
	Title    string
	Url      string
	Selector string
}

func main() {
	// display the message to the user
	fmt.Println("enter the website url and its corresponding html selector separated by space to scrape content from (one per line)\nexample: https://www.example.com #content")
	fmt.Println("to start scraping type 'start'. to exit type 'exit' or press 'ctrl + c'")

	scanner := bufio.NewScanner(os.Stdin)

	websites := make(map[string]string)
	scrapeResults := make(chan ScrapeResult)

	// read input from the user
	for {
		fmt.Print("website url and html selector: ")
		scanner.Scan()
		input := scanner.Text()

		if input == strings.TrimSpace("exit") {
			log.Println("stoping the scrapper...")
			os.Exit(0)
		}

		if input == strings.TrimSpace("start") {
			log.Println("starting the scrapper...")
			break
		}

		inputVals := strings.Fields(input)

		if len(inputVals) < 2 || len(inputVals) > 2 {
			log.Println("invalid input. please enter the website url and its corresponding html selector separated by space")
			continue
		} else {
			url, err := url.Parse(inputVals[0])
			if err != nil || url.Scheme == "" || url.Host == "" {
				log.Println("error parsing the website url:", err)
				continue
			} else {
				websites[inputVals[0]] = inputVals[1]
			}
		}

	}

	// create a wait group to sync the goroutines
	var wg sync.WaitGroup

	// iterate over websites and scrape the content
	for url, selector := range websites {
		wg.Add(1)

		go func(url, selector string) {
			defer wg.Done()

			// scrape the website content
			scrapeWebsite(url, selector, scrapeResults)
		}(url, selector)
	}

	go func() {
		// wait for all the goroutines to finish and close the channel
		wg.Wait()
		close(scrapeResults)
	}()

	// iterate over the scrape results
	for result := range scrapeResults {
		fmt.Printf("scrape result: %+v\n", result)
	}
}

func scrapeWebsite(url, selector string, results chan<- ScrapeResult) {
	log.Printf("scraping the website: %s\n", url)

	// fetch the website content
	resp, err := http.Get(url)
	if err != nil {
		log.Println("error fetching the website:", err)
		return
	}
	defer resp.Body.Close()

	// parse the html content
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		log.Println("error parsing the html content:", err)
	}

	// extract doc title
	title := doc.Find("title").Text()

	results <- ScrapeResult{
		Title:    title,
		Url:      url,
		Selector: selector,
	}
}
