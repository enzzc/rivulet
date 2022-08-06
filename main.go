package main

import (
	"context"
	"html/template"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"
)

const (
	redisPrefix = "riv:"
	separator   = "^"
	allFeedsKey = "feeds-all"
)

const (
	maxWaitMsecs      = 1500000 // 25 min
	waitIntervalHours = 2
	maxItemsToKeep    = 128
	maxAgeDays        = 90 // ~3 months
)

const (
	userAgent        = "Rivulet-Bot Feed Fetcher [RSS/Atom/JSON Feed]"
	fetchTimeoutSecs = 15
)

var logger *zap.SugaredLogger

type Post struct {
	Id       string `json:"id"`
	Title    string `json:"title"`
	Link     string `json:"link"`
	Pubdate  string `json:"pubdate"`
	Domain   string `json:"domain"`
	FeedLink string `json:"feedlink"`
	Clap     bool   `json:"clap"`
}

func (p *Post) ShortDateDisplay() string {
	return strings.Split(p.Pubdate, " +")[0]
}

func (p *Post) ShortDomainDisplay() string {
	url, err := url.Parse(p.Link)
	if err != nil {
		return p.Domain
	}
	return strings.TrimPrefix(url.Host, "www.")
}

func init() {
	rand.Seed(time.Now().UnixNano())
	l, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	logger = l.Sugar()
}

func main() {
	go func() {
		for {
			for !fetchNewFeeds() {
				logger.Infow("No feed to fetch yet.")
				time.Sleep(10 * time.Second)
			}
			trim(redisPrefix+allFeedsKey, maxItemsToKeep)
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
		logger.Infow("Fetching",
			"count", len(list),
		)
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

func trim(key string, keep int) error {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	defer rdb.Close()
	trimmed, err := rdb.ZRemRangeByRank(context.Background(), key, 0, -1*int64(keep)).Uint64()
	logger.Infow("Trimmed",
		"count", trimmed,
		"key", key,
	)
	return err
}

func parseAndInsert(feedUrl string) error {
	parsedURL, err := url.Parse(feedUrl)
	if err != nil {
		logger.Warnw("ParseAndInsert",
			"url", feedUrl,
			"err", err,
		)
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
			logger.Warnw("HTTPHead",
				"url", feedUrl,
				"err", err,
			)
			return err
		} else if resp.StatusCode == http.StatusNotModified {
			logger.Infow("HTTPHead",
				"url", feedUrl,
				"http_code", resp.StatusCode,
			)
			return nil
		}
	}

	// No Etag or mismatched Etag
	logger.Infow("HTTPGet",
		"url", feedUrl,
	)

	req, err := http.NewRequest("GET", feedUrl, nil)
	req.Header.Add("User-Agent", userAgent)
	resp, err := client.Do(req)

	if err != nil {
		logger.Warnw("HTTPGet",
			"url", feedUrl,
			"err", err,
		)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		newEtag := resp.Header.Get("Etag")
		if len(newEtag) > 0 {
			logger.Infow("NewEtag",
				"url", feedUrl,
				"new_etag", newEtag,
			)
			rdb.HSet(ctx, cacheKey, "etag", newEtag, "last", time.Now().UTC().Unix())
		}
	} else {
		logger.Warnw("NewEtag",
			"url", feedUrl,
			"http_code", resp.StatusCode,
		)
		return nil
	}

	feed, err := fp.Parse(resp.Body)
	if err != nil {
		logger.Warnw("Parsing",
			"url", feedUrl,
			"err", err,
		)
		return err
	}

	for _, item := range feed.Items {
		date := item.PublishedParsed
		if date == nil {
			logger.Warnw("SkipLink",
				"url", feedUrl,
				"item_link", item.Link,
				"item_published", item.Published,
			)
			continue
		}
		if date.Before(time.Now().Add(-maxAgeDays * 24 * time.Hour)) {
			// Older than maxAgeDays, skip
			continue
		}
		score := date.UTC().Unix()
		entry := feedId + separator + item.Link + separator + item.Title
		redisZEntry := &redis.Z{float64(score), entry}

		rdb.Pipelined(ctx, func(rdb redis.Pipeliner) error {
			rdb.ZAdd(ctx, redisPrefix+"feed:"+feedUrl, redisZEntry)
			rdb.ZAdd(ctx, redisPrefix+allFeedsKey, redisZEntry)
			return nil
		})
		logger.Infow("Inserted",
			"url", feedUrl,
			"item_link", item.Link,
			"item_published", item.PublishedParsed,
		)

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

	var lim int64 = maxItemsToKeep
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
	w.Header().Set("Cache-Control", "public, max-age=900, must-revalidate") // cache 15 min
	t := template.Must(template.ParseFiles("index.tpl"))
	t.Execute(w, struct {
		Items  *[]*Post
		Domain string
	}{&posts, domain})
}
