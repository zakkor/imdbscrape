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
	scrapeType = flag.String("scrape", "", "type of page to scrape")
	f          = flag.String("f", "", "file to get data from, for multiple* scrape kinds")
)

var (
	byActorTemplate = template.Must(template.New("byActor").Parse(
		`https://www.imdb.com/filmosearch/?explore=title_type&role={{ index . "id" }}&ref_=filmo_nxt&mode=simple&page=%d&sort={{ index . "sort" }},{{ index . "sortOrder" }}&title_type={{ index . "titleType" }}`,
	))
	listTemplate = template.Must(template.New("list").Parse(
		`https://www.imdb.com/list/{{ index . "id" }}/?sort=list_order,asc&mode=detail&page=%d`,
	))
	firstNumber = regexp.MustCompile(`\d+`)
	nameID      = regexp.MustCompile(`nm\d+`)
)

type scrapeKind string

const (
	scrapeKindActorMovies     scrapeKind = "actormovies"
	scrapeKindManyActorMovies scrapeKind = "manyactormovies"
	scrapeKindListActors      scrapeKind = "listactors"
)

type movie struct {
	Title  string  `json:"title"`
	Year   int     `json:"year"`
	Rating float32 `json:"rating"`
}

type actor struct {
	Name   string `json:"name"`
	ImdbID string `json:"imdb_id"`
}

type actormovies struct {
	Actor  actor   `json:"actor"`
	Movies []movie `json:"movies"`
}

func main() {
	flag.Parse()

	if *scrapeType == "" {
		log.Fatalln("must specify -scrape")
	}

	if scrapeKind(*scrapeType) == scrapeKindManyActorMovies {
		if *f == "" {
			log.Fatalln("must specify -f, which should be a file containing a JSON list of actors")
		}
	} else {
		if *id == "" {
			log.Fatalln("must specify -id")
		}
	}

	// Instantiate default collector
	c := colly.NewCollector(
		colly.AllowedDomains("www.imdb.com"),
	)

	// Before making a request
	c.OnRequest(func(r *colly.Request) {
		// Set preferred language.
		r.Headers.Set("Accept-Language", "en-US,en;q=0.9,ro;q=0.8")

		// Print "Visiting ..."
		log.Println("Visiting", r.URL.String())
	})

	switch scrapeKind(*scrapeType) {
	case scrapeKindActorMovies:
		scrapeActorMovies(c, *id)

	case scrapeKindManyActorMovies:
		// f must be a file containing JSON-encoded []actor
		data, err := ioutil.ReadFile(*f)
		if err != nil {
			log.Fatalln(err)
		}
		var actors []actor
		err = json.Unmarshal(data, &actors)
		if err != nil {
			log.Fatalln(err)
		}

		// c.Async = true
		for _, a := range actors {
			scrapeActorMovies(c, a.ImdbID)
		}
		// c.Wait()

	case scrapeKindListActors:
		urlfmt := executeURLTemplate(listTemplate, map[string]string{"id": *id})
		scrapeListActors(c, urlfmt)
	}
}

func scrapeActorMovies(c *colly.Collector, id string) {
	urlfmt := executeURLTemplate(byActorTemplate, map[string]string{
		"id":        id,
		"sort":      "year",
		"sortOrder": "asc",
		"titleType": "movie",
	})

	// Will be set to true when we need to stop scraping
	var stop bool
	var p int = 1
	var am actormovies

	c.OnHTML(".article h1.header", func(e *colly.HTMLElement) {
		if am.Actor.Name != "" {
			return
		}

		text := strings.TrimSpace(e.Text)

		i := strings.Index(text, "With")
		if i == -1 {
			log.Fatalln("can't find actor name:", id)
			return
		}

		name := text[i+len("With")+1:]
		am.Actor.Name = name
		am.Actor.ImdbID = id
	})

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

		m := movie{
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
			m.Year = int(yearInt)
		}

		if rating != "" {
			ratingFloat, err := strconv.ParseFloat(rating, 32)
			if err != nil {
				log.Printf("error: cannot convert rating %s to float32: %s", rating, err.Error())
				return
			}
			m.Rating = float32(ratingFloat)
		}

		am.Movies = append(am.Movies, m)
		save("./scraped/actormovies/actormovies-"+id+".json", am)
	})

	c.OnScraped(func(r *colly.Response) {
		if stop {
			return
		}

		// Visit next page
		p++
		c.Visit(page(urlfmt, p))
	})

	// Start scraping.
	c.Visit(page(urlfmt, p))
}

func scrapeListActors(c *colly.Collector, urlfmt string) {
	// Will be set to false if something is found
	var found bool
	var p int = 1
	var actors []actor

	c.OnHTML(".lister-item-header a", func(e *colly.HTMLElement) {
		found = true
		name := strings.TrimSpace(e.Text)
		imdbID := nameID.FindString(e.Attr("href"))

		actors = append(actors, actor{
			Name:   name,
			ImdbID: imdbID,
		})
		save("./scraped/listactors/listactors-"+*id+".json", actors)
	})

	c.OnScraped(func(r *colly.Response) {
		if !found {
			return
		}

		// Reset found
		found = false
		// Visit next page
		p++
		c.Visit(page(urlfmt, p))
	})

	// Start scraping.
	c.Visit(page(urlfmt, p))
}

func page(format string, n int) string {
	return fmt.Sprintf(format, n)
}

func save(filename string, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Println("error: could not marshal to JSON:", err)
		return
	}

	err = ioutil.WriteFile(filename, data, 0644)
	if err != nil {
		log.Printf("error: could not write file \"%v\": %s\n", filename, err.Error())
	}
}

func executeURLTemplate(t *template.Template, args map[string]string) string {
	var buf bytes.Buffer
	err := t.Execute(&buf, args)
	if err != nil {
		log.Fatalln(err)
	}
	return buf.String()
}
