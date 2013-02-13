// Should probably be its own package, but this is a minimal PoC.
// Names are capitalized as if this was a separate package.
package main

import (
    "net/http"
    "net/url"
    "sync"
    "log"
    "io/ioutil"
)

type Crawler struct {
    // For this minimal case we'll just keep all results in memory with the
    // results-map, but in a real-world scenario we'd want to dump results
    // to disk at least every now and then. Without dumping to disk the
    // memory consumption of the crawler will steadily go up unchecked.
    results map[string]*result
    // Since multiple worker goroutines will be working with the result-map
    // at the same time we need a mutex to lock it while in use.
    sync.Mutex
}

type result struct {
    response string
    urls     []string
}


func NewCrawler(seedUrl string) *Crawler {
    crawler := &Crawler{results: make(map[string]*result)}
    urlQueue := make(chan string, 10000) // Rather arbitrary buffer-size.

    // Spinning up a number of worker-goroutines. We could alternatively
    // choose to spin up a new goroutine for every new url discovered, but
    // we would risk running out of memory on some systems, so a pool of
    // a manageable number of goroutines is kept. The number should be chosen
    // so as to maximize throughput while minimizing memory-utilization, and
    // we would need some epirical testing to figure out the optimum for a
    // given system.
    for w:= 1; w <= 100; w++ {
        go worker(urlQueue, crawler)
    }

    // TODO Determine when to quit somehow.
    // Depth or through a special quit-channel.
}

func worker(queue chan string, crawler *Crawler) {
    for url := range queue {
        *crawler.Lock()
        if _, ok := crawler.results[url]; ok {
            // url already in result-map.
            *crawnler.Unlock()
            continue
        }

        response, err := http.Get(url)
        if err != nil {
            // We should probably handle som errors more graciously, since not
            // all strings on the web are alive and well.
            log.Fatal(err)
        }

        body, err := ioutil.ReadAll(response.Body)
        if err != nil {
            log.Fatal(err)
        }
        response.Body.Close()

        urls := getUrls(body)

        // Add the new urls we found to the queue.
        for newUrl := range urls {
            queue<-newUrl
        }

        // We have to do this check again in the offchance that some other
        // goroutine has processed the same url since the last time we checked.
        *crawler.Lock()
        if _, ok := crawler.results[url]; ok {
            *crawnler.Unlock()
            continue
        }
        crawler.results[url] = &result{response: body, urls: urls}
        *crawler.Unlock()
    }
}

