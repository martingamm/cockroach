// Should probably be its own package, but this is a minimal PoC.
// Names are capitalized as if this was a separate package.
package main

import (
    "net/http"
    "sync"
    "log"
    "io/ioutil"
)

type Result struct {
    documentUrl string
    // We store http.Responses instead of the body string or similar because
    // http.Results are standardized and easy to work with in Go, if this
    // crawler was to be used as a library by some other application.
    response    *http.Response
    urls        []string
}

type queueItem struct {
    url   string
    depth int
}

type outputmap struct {
    // For this minimal case we'll just keep all results in memory with the
    // results-map, but in a real-world scenario we'd want to dump results
    // to disk at least every now and then. Without dumping to disk the
    // memory consumption of the crawler will steadily go up unchecked.
    results map[string]*Result
    sync.Mutex
}

func Crawl(seedUrl string, maxDepth int) map[string]*Result {
    output := outputmap{results: make(map[string]*Result)}
    urlChan := make(chan queueItem, 100) // Rather arbitrary buffer-sizes.
    resultChan := make(chan *Result, 100)

    // Spinning up a number of worker-goroutines. We could alternatively
    // choose to spin up a new goroutine for every new url discovered, but
    // we would risk running out of memory on some systems, so a pool of
    // a manageable number of goroutines is kept. The number should be chosen
    // so as to maximize throughput while minimizing memory-utilization, and
    // we would need some epirical testing to figure out the optimum for a
    // given system.
    for w:= 1; w <= 10; w++ {
        go worker(urlChan, resultChan, maxDepth, output)
    }

    urlChan<-queueItem{url:seedUrl, depth: 0}
    

    for res := range resultChan {
        // The key (url) is now redundant since it serves as key AND as part
        // of the result. Done this way to simplify the results-channel so we
        // don't need another type (no tuples in Go) just for this.
        // The overhead is rather small anyway, but this could of course be
        // optimized if the need arises.
        output.Lock()
        output.results[res.documentUrl] = res
        output.Unlock()
    }

    // maps in Go are a reference-type, so no use returning a pointer.
    return output.results
}

func worker(queue chan queueItem, results chan *Result, maxDepth int, output outputmap) {
    for item := range queue {
        // If the url is in the result-set already, we skip ahead.
        output.Lock()
        if _, ok := output.results[item.url]; ok {
            output.Unlock()
            continue
        }
        output.Unlock()

        response, err := http.Get(item.url)
        // Something wrong happened to the request, log what went wrong and go
        // to next url in queue.
        if err != nil {
            log.Print(err)
            continue
        }

        body, err := ioutil.ReadAll(response.Body)
        if err != nil {
            log.Print(err)
        }
        response.Body.Close()

        urls := getUrls(string(body))

        // We set documentUrl to the url supplied instead of the actual url
        // (which might be different after redirects) to maintain tree-structure
        // in the result set.
        results<-&Result{documentUrl: item.url, response: response, urls: urls}

        // The first worker to hit maxDepth closes the queues, forcing all
        // workers to quit after their current processing and the main Crawl
        // goroutine to pop out of the result-gathering loop.
        if item.depth >= maxDepth {
            close(queue)
            close(results)
            return
        }

        // Add the new urls we found to the queue.
        for _, newUrl := range urls {
            queue<-queueItem{url:newUrl, depth:item.depth+1}
        }
    }

    return
}

// TODO
func getUrls(body string) []string {
    return []string{"http://comoyo.com", "http://djuice.no"}
}
