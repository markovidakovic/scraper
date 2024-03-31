package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

const (
	dirResults string = "scrape-results"
)

var (
	outputFileFormat string
	downloadImages   bool
)

type ScrapeResult struct {
	Title    string   `json:"title" xml:"title"`
	Url      string   `json:"url" xml:"url"`
	Selector string   `json:"selector" xml:"selector"`
	ImgUrls  []string `json:"img_urls" xml:"imgUrls>imgUrl"`
}

func main() {
	// scraped results file format flag
	flag.StringVar(&outputFileFormat, "output-file-format", "txt", "scraped results file format (txt, csv, json, xml)")
	flag.BoolVar(&downloadImages, "download", false, "download the scraped images")
	flag.Parse()

	// display the message to the user
	fmt.Println("enter the website url and its corresponding html selector separated by a comma to scrape content from (one per line)\nexample: https://www.example.com,div.product-image img")
	fmt.Println("to start scraping type 'start'. to exit type 'exit' or press 'ctrl + c'")

	scanner := bufio.NewScanner(os.Stdin)

	websites := make(map[string]string)
	scrapeResults := make(chan ScrapeResult)

	// read input from the user
	for {
		fmt.Print("website url and html selector: ")
		scanner.Scan()
		input := scanner.Text()

		input = strings.TrimSpace(input)

		if input == "exit" {
			log.Println("stoping the scrapper...")
			os.Exit(0)
		}

		if input == "start" {
			log.Println("starting the scrapper...")
			break
		}

		inputVals := strings.Split(input, ",")

		url, err := url.Parse(inputVals[0])
		if err != nil || url.Scheme == "" || url.Host == "" {
			log.Println("error parsing the website url:", err)
			continue
		} else {
			websites[inputVals[0]] = inputVals[1]
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

	err := os.MkdirAll(dirResults, 0755)
	if err != nil {
		log.Println("error creating the srape results directory:", err)
	}

	// iterate over the scrape results
	for result := range scrapeResults {
		wg.Add(1)
		go func(result ScrapeResult) {
			defer wg.Done()
			handleScrapeResult(result)
		}(result)
	}
	wg.Wait()
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

	// extract the desired content from the website (images in this case)
	elems := doc.Find(selector)

	// slice of image urls
	imgUrls := make([]string, 0)

	elems.Each(func(i int, elem *goquery.Selection) {
		val, exists := elem.Attr("src")
		if exists {
			// perform a http head request to the image url to check if it's valid
			resp, err := http.Head(val)
			if err != nil {
				log.Printf("error checking the image url %s: %v\n", val, err)
			}
			defer resp.Body.Close()

			// check if response status code is in the 2xx range
			if resp.StatusCode >= 200 || resp.StatusCode < 300 {
				// check if the content type is an image
				contentType := resp.Header.Get("Content-Type")

				if contentType == "image/jpeg" || contentType == "image/png" || contentType == "image/gif" {
					imgUrls = append(imgUrls, val)
				} else {
					log.Printf("image url %s is not an image\n", val)
				}
			} else {
				log.Printf("image url %s is not valid\n", val)
			}
		}
	})

	results <- ScrapeResult{
		Title:    title,
		Url:      url,
		Selector: selector,
		ImgUrls:  imgUrls,
	}
}

func handleScrapeResult(result ScrapeResult) {
	url, err := url.Parse(result.Url)
	if err != nil {
		log.Println("error parsing the website url:", err)
		return
	}

	fldrName := strings.Join(strings.Split(url.Host, "."), "-")
	fileName := strings.TrimSuffix(strings.Join(strings.Split(url.Path[1:], "/"), "-"), "-")

	fldrPath := path.Join(dirResults, fldrName)

	err = os.MkdirAll(fldrPath, 0755)
	if err != nil {
		log.Println("error creating the website directory:", err)
		return
	}

	filePath := fmt.Sprintf("%s/%s/%s.%s", dirResults, fldrName, fileName, outputFileFormat)

	file, err := os.Create(filePath)
	if err != nil {
		log.Println("error creating the file:", err)
		return
	}
	defer file.Close()

	switch outputFileFormat {
	case "json":
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", " ")
		err := encoder.Encode(result)
		if err != nil {
			log.Println("error encoding the scrape result:", err)
			return
		}
	case "csv":
		writer := csv.NewWriter(file)
		defer writer.Flush()

		// write header
		writer.Write([]string{"Title", "URL", "Selector", "Image URLs"})

		// write data
		for _, imgUrl := range result.ImgUrls {
			writer.Write([]string{result.Title, result.Url, result.Selector, imgUrl})
		}
	case "xml":
		xmlData, err := xml.MarshalIndent(result, "", "  ")
		if err != nil {
			log.Println("error marshaling the xml scrape result:", err)
			return
		}
		_, err = file.Write(xmlData)
		if err != nil {
			log.Println("error writing the xml scrape result:", err)
			return
		}
	default:
		fmt.Fprintf(file, "Title: %s\n\n", result.Title)
		fmt.Fprintf(file, "URL: %s\n\n", result.Url)
		fmt.Fprintf(file, "Selector: %s\n\n", result.Selector)
		fmt.Fprint(file, "Image URLs:\n\n")
		for _, imgUrl := range result.ImgUrls {
			fmt.Fprintf(file, "- %s\n", imgUrl)
		}
	}

	if downloadImages {
		// the fileName var will be used as a folder name for the images
		fldrImages := path.Join(fldrPath, fileName)
		err := os.MkdirAll(fldrImages, 0755)
		if err != nil {
			log.Println("error creating the images directory:", err)
			return
		}

		var wg sync.WaitGroup

		for _, imgUrl := range result.ImgUrls {
			wg.Add(1)

			imgName := path.Base(imgUrl)
			imgExt := path.Ext(imgName)
			imgName = strings.TrimSuffix(imgName, imgExt)

			go func(imgUrl, imgName, imgExt, fldrImages string) {
				defer wg.Done()

				// fetch the image
				resp, err := http.Get(imgUrl)
				if err != nil {
					log.Println("error downloading the image:", err)
					return
				}
				defer resp.Body.Close()

				// check the response status code
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					log.Printf("error downloading the image %s: %s\n", imgName, resp.Status)
					return
				}

				// create the image file
				imgFile, err := os.Create(fmt.Sprintf("%s/%s%s", fldrImages, imgName, imgExt))
				if err != nil {
					log.Println("error creating the image file:", err)
					return
				}
				defer imgFile.Close()

				// copy response body (image data) to the file
				_, err = io.Copy(imgFile, resp.Body)
				if err != nil {
					log.Println("error copying the image data to the file:", err)
					return
				}
			}(imgUrl, imgName, imgExt, fldrImages)
		}

		wg.Wait()
		log.Printf("scrape result images saved to folder: %s\n", fldrImages)
	}

	log.Printf("scrape result saved to file: %s\n", filePath)
}
