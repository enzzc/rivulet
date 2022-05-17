package main

import (
	"context"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/mmcdole/gofeed"
)

const redisPrefix = "riv:"
const separator = "^"

const (
	maxWaitMsecs      = 1500000 // 25 min
	waitIntervalHours = 2
)

const (
	userAgent        = "Rivulet-Bot Feed Fetcher [RSS/Atom/JSON Feed]"
	fetchTimeoutSecs = 15
)

type Post struct {
	Id       string `json:"id"`
	Title    string `json:"title"`
	Link     string `json:"link"`
	Pubdate  string `json:"pubdate"`
	Domain   string `json:"domain"`
	FeedLink string `json:"feedlink"`
	Clap     bool   `json:"clap"`
}

func main() {
	rand.Seed(time.Now().UnixNano())
	go func() {
		for {
			for !fetchNewFeeds() {
				log.Println("No feed to fetch yet.")
				time.Sleep(10 * time.Second)
			}
			time.Sleep(waitIntervalHours * time.Hour)
		}
	}()
	http.HandleFunc("/", apiHandler)
	http.ListenAndServe(":8080", nil)
}

func fetchNewFeeds() bool {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	list, _ := rdb.SMembers(ctx, redisPrefix+"feeds").Result()
	rdb.Close()

	if len(list) > 0 {
		log.Println("Fetching", len(list), "feeds")
	} else {
		return false
	}

	var wg sync.WaitGroup
	for _, feedUrl := range list {
		url := feedUrl
		wg.Add(1)
		go func() {
			defer wg.Done()
			wait := time.Duration(rand.Intn(maxWaitMsecs)) * time.Millisecond
			time.Sleep(wait)
			parseAndInsert(url)
		}()
	}
	wg.Wait()
	return true
}

func parseAndInsert(feedUrl string) error {
	parsedURL, err := url.Parse(feedUrl)
	if err != nil {
		log.Println(feedUrl, err)
		return err
	}
	feedId := parsedURL.Host + parsedURL.Path
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	defer rdb.Close()

	fp := gofeed.NewParser()

	cacheKey := redisPrefix + "cache:" + feedUrl
	etag, _ := rdb.HGet(ctx, cacheKey, "etag").Result()

	client := &http.Client{
		Timeout: fetchTimeoutSecs * time.Second,
	}
	defer client.CloseIdleConnections()

	// If Etag is present, skip if it's matching
	if len(etag) > 0 {
		req, err := http.NewRequest("HEAD", feedUrl, nil)
		req.Header.Add("User-Agent", userAgent)
		req.Header.Add("If-None-Match", etag)
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("HEAD %s ERROR: %v\n", feedUrl, err)
			return err
		} else if resp.StatusCode == http.StatusNotModified || resp.Header.Get("Etag") == etag {
			log.Printf("HEAD %s returned %d, not modified: skip.\n", feedUrl, resp.StatusCode)
			return nil
		}
	}

	// No Etag or mismatched Etag
	log.Println("GET", feedUrl)

	req, err := http.NewRequest("GET", feedUrl, nil)
	req.Header.Add("User-Agent", userAgent)
	resp, err := client.Do(req)

	if err != nil {
		log.Printf("GET %s ERROR: %v\n", feedUrl, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		newEtag := resp.Header.Get("Etag")
		if len(newEtag) > 0 {
			log.Println(feedUrl, "New Etag:", newEtag)
			rdb.HSet(ctx, cacheKey, "etag", newEtag, "last", time.Now().UTC().Unix())
		}
	} else {
		log.Println(feedUrl, "HTTP: returned", resp.StatusCode)
		return nil
	}

	feed, err := fp.Parse(resp.Body)
	if err != nil {
		log.Println(feedUrl, "Parsing Error:", err)
		return err
	}

	for _, item := range feed.Items {
		date := item.PublishedParsed
		if date == nil {
			log.Println(feedUrl, item.Link, item.Published)
			continue
		}
		score := date.UTC().Unix()
		entry := feedId + separator + item.Link + separator + item.Title
		redisZEntry := &redis.Z{float64(score), entry}

		rdb.Pipelined(ctx, func(rdb redis.Pipeliner) error {
			rdb.ZAdd(ctx, redisPrefix+"feed:"+feedUrl, redisZEntry)
			rdb.ZAdd(ctx, redisPrefix+"feeds-all", redisZEntry)
			return nil
		})
	}
	return nil
}

/* HTTP handlers */

func apiHandler(w http.ResponseWriter, req *http.Request) {
	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	defer rdb.Close()

	var lim int64 = 128
	fromScore := "0"
	toScore := strconv.Itoa(int(time.Now().Unix()))

	var zslice []redis.Z
	var domain string
	q := req.URL.Query()
	var key string
	if len(q["domain"]) > 0 {
		domain = q["domain"][0]
		key = redisPrefix + "feed:" + domain
	} else {
		key = redisPrefix + "feeds-all"
	}
	zslice, _ = rdb.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{fromScore, toScore, 0, lim}).Result()

	posts := make([]*Post, 0, 64)
	for _, zrec := range zslice {
		parts := strings.Split(zrec.Member.(string), separator)
		domain := parts[0]
		link := parts[1]
		title := parts[2]
		published := time.Unix(int64(zrec.Score), 0).UTC()
		posts = append(posts, &Post{link, title, link, published.String(), domain, domain, false})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300, must-revalidate") // cache 5 min
	t := template.Must(template.ParseFiles("index.tpl"))
	t.Execute(w, struct {
		Items  *[]*Post
		Domain string
	}{&posts, domain})
}
