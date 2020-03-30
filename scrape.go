package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/gocolly/colly"
)

type Entry struct {
	Title  string  `json:"title"`
	Year   int     `json:"year"`
	Rating float32 `json:"rating"`
}

var id = flag.String("id", "", "")

func main() {
	flag.Parse()
	if *id == "" {
		log.Fatalln("must specify -id")
	}
	var p int = 1
	var entries []Entry
	var stop bool

	// Instantiate default collector
	c := colly.NewCollector(
		colly.AllowedDomains("www.imdb.com"),
	)

	c.OnHTML(".lister-item", func(e *colly.HTMLElement) {
		if strings.TrimSpace(e.Text) == "No results. Try removing genres, ratings, or other filters to see more." {
			stop = true
		}
	})
	c.OnHTML(".lister-col-wrapper", func(e *colly.HTMLElement) {
		if stop {
			return
		}
		rating := e.ChildText(".col-imdb-rating strong")
		title := e.ChildText(".col-title span[title] > a:first-child")
		year := e.ChildText(".col-title span[title] > .lister-item-year")
		reYear := regexp.MustCompile(`\d+`)
		year = reYear.FindString(year)

		entry := Entry{
			Title: title,
		}

		if year != "" {
			yearInt, err := strconv.ParseInt(year, 10, 32)
			if err != nil {
				log.Println("error converting year", year, "to int:", err)
			}
			entry.Year = int(yearInt)
		} else {
			entry.Year = -1
		}

		if rating != "" {
			ratingFloat, err := strconv.ParseFloat(rating, 32)
			if err != nil {
				log.Println("error converting rating", rating, "to float32:", err)
			}
			entry.Rating = float32(ratingFloat)
		} else {
			entry.Rating = -1
		}

		entries = append(entries, entry)
		save(entries)
	})
	c.OnScraped(func(r *colly.Response) {
		if stop {
			return
		}

		p++
		c.Visit(page(*id, p))
	})

	// Before making a request print "Visiting ..."
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9,ro;q=0.8")
		fmt.Println("Visiting", r.URL.String())
	})

	c.Visit(page(*id, p))
}

func page(id string, n int) string {
	const format = "https://www.imdb.com/filmosearch/?explore=title_type&role=%s&ref_=filmo_nxt&mode=simple&page=%v&sort=year,desc&title_type=movie"
	return fmt.Sprintf(format, id, n)
}

func save(entries []Entry) {
	data, err := json.Marshal(entries)
	if err != nil {
		panic(err)
	}
	err = ioutil.WriteFile(*id+".json", data, 0644)
	if err != nil {
		panic(err)
	}
}
