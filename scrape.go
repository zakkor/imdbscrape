package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/gocolly/colly"
)

var (
	id         = flag.String("id", "", "id to scrape")
	scrapeType = flag.String("t", "actormovies", "type of page to scrape")
)

var (
	byActorTemplate = template.Must(template.New("").Parse(
		`https://www.imdb.com/filmosearch/?explore=title_type&role={{ index . "id" }}&ref_=filmo_nxt&mode=simple&page=%d&sort={{ index . "sort" }},{{ index . "sortOrder" }}&title_type={{ index . "titleType" }}`,
	))
	urls = map[string]*template.Template{
		"actormovies": byActorTemplate,
	}
	firstNumber = regexp.MustCompile(`\d+`)
)

type Entry struct {
	Title  string  `json:"title"`
	Year   int     `json:"year"`
	Rating float32 `json:"rating"`
}

func main() {
	flag.Parse()

	if *id == "" {
		log.Fatalln("must specify -id")
	}

	var (
		// Page count
		p int = 1
		// Will be set to true when we need to stop scraping
		stop bool
		// List of entries that will be serialized to JSON
		entries []Entry
	)

	urlTmpl, ok := urls[*scrapeType]
	if !ok {
		log.Fatalf("scrapeType %s does not exist\n", *scrapeType)
	}

	var urlfmtBuf bytes.Buffer
	err := urlTmpl.Execute(&urlfmtBuf, map[string]string{
		"id":        *id,
		"sort":      "year",
		"sortOrder": "asc",
		"titleType": "movie",
	})
	if err != nil {
		log.Fatalln(err)
	}
	urlfmt := urlfmtBuf.String()

	// Instantiate default collector
	c := colly.NewCollector(
		colly.AllowedDomains("www.imdb.com"),
	)

	// Check to see if we should stop.
	c.OnHTML(".lister-item", func(e *colly.HTMLElement) {
		if strings.TrimSpace(e.Text) == "No results. Try removing genres, ratings, or other filters to see more." {
			stop = true
		}
	})

	c.OnHTML(".lister-col-wrapper", func(e *colly.HTMLElement) {
		// Stop early
		if stop {
			return
		}

		rating := e.ChildText(".col-imdb-rating strong")
		title := e.ChildText(".col-title span[title] > a:first-child")
		year := e.ChildText(".col-title span[title] > .lister-item-year")
		year = firstNumber.FindString(year)

		entry := Entry{
			Title:  title,
			Year:   -1, // Year not known yet
			Rating: -1, // No rating yet
		}

		if year != "" {
			yearInt, err := strconv.ParseInt(year, 10, 32)
			if err != nil {
				log.Printf("error: cannot convert year %s to int32: %s", year, err.Error())
				return
			}
			entry.Year = int(yearInt)
		}

		if rating != "" {
			ratingFloat, err := strconv.ParseFloat(rating, 32)
			if err != nil {
				log.Printf("error: cannot convert rating %s to float32: %s", rating, err.Error())
				return
			}
			entry.Rating = float32(ratingFloat)
		}

		entries = append(entries, entry)
		save(*id+".json", entries)
	})
	c.OnScraped(func(r *colly.Response) {
		if stop {
			return
		}

		// Visit next page
		p++
		c.Visit(page(urlfmt, p))
	})

	// BEFORE making a request
	c.OnRequest(func(r *colly.Request) {
		// Set preferred language.
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9,ro;q=0.8")

		// Print "Visiting ..."
		log.Println("Visiting", r.URL.String())
	})

	// Start scraping.
	c.Visit(page(urlfmt, p))
}

func page(format string, n int) string {
	return fmt.Sprintf(format, n)
}

func save(filename string, entries []Entry) {
	data, err := json.Marshal(entries)
	if err != nil {
		log.Println("error: could not marshal entries to JSON:", err)
		return
	}

	err = ioutil.WriteFile(filename, data, 0644)
	if err != nil {
		log.Printf("error: could not write file \"%v\": %s\n", filename, err.Error())
	}
}
