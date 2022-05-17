# Rivulet

Rivulet is a simple and (very) minimalist personal Web-based RSS/Atom/JSON Feed
aggregator. It's written in Go and uses Redis to keep and index data (which is
transient anyway).

Here is a running instance: <https://rivulet.sagebl.eu>.

## (Non)features
 * Fast and light.
 * Visited links are purple, i.e., no need for a read/not-read flag.
 * No "save" feature to read later. (i.e., never.)
 * No preview: the title of the entry is enough.
 * No tags or categories: it's just a continuous rivulet of links.
 * Only one binary handling the Web service and the fetching.
 * Very minimal CSS styling, yet responsive.
 * Checks Etags to reduce network usage when available.
 * Spreads fetching processes to reduce network bursts.

## How to use
 * Build Rivulet: `make rivulet`
 * Run Redis
 * Run Rivulet
 * Populate Redis with your feeds, e.g.: `SADD riv:feeds "https://example.com/feed.atom"`

If you have a list in a plain text format, here is a trick to populate Redis:
```bash
cat feeds.txt | xargs printf "sadd riv:feeds '%s'\n" | redis-cli --pipe
```

## Dependencies:
 * <https://github.com/go-redis/redis/>
 * <https://github.com/mmcdole/gofeed>
