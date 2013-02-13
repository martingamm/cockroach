// TODO: Remove logs and debugs.
// TODO: Add quit after x seconds if depth not reached.
// TODO:
// TODO:

// Should probably be its own package, but this is a minimal PoC.
// Names are capitalized as if this was a separate package.
package main

import (
    "net/http"
    "net/url"
    "sync"
    "log"
    "io/ioutil"
    "regexp"
    "runtime"
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
    // memory consumption of the crawler will steadily rise unchecked.
    results map[string]*Result
    // All the goroutines including the main Crawl goroutine will be
    // accessing the result-map, so we need a way to lock it.
    sync.Mutex
}

func Crawl(seedUrl string, maxDepth, nWorkers int) map[string]*Result {
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
    for w:= 1; w <= nWorkers; w++ {
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

        //log.Print("result saved: ", res.documentUrl, len(output.results))
    }

    // maps in Go are a reference-type, so no use returning a pointer.
    return output.results
}

func worker(queue chan queueItem, results chan *Result, maxDepth int, output outputmap) {
    for item := range queue {
        log.Print("got new url from queue: ", item.url, " at depth ", item.depth)
        //log.Print("worker began on: ", item.url, ", depth: ", item.depth)
        // If the url is in the result-set already, we skip ahead.
        output.Lock()
        if _, ok := output.results[item.url]; ok {
            output.Unlock()
            //log.Print("worker skipping: ", item.url)
            continue
        }
        // Put placeholder so other workers correctly skip.
        output.results[item.url] = &Result{}
        output.Unlock()

        response, err := http.Get(item.url)
        // Something wrong happened to the request, log what went wrong and go
        // to next url in queue.
        if err != nil {
            //log.Print(err, "continued loopWASDF")
            continue
        }

        urls := getUrls(response, item.url)

        // We set documentUrl to the url supplied instead of the actual url
        // (which might be different after redirects) to maintain tree-structure
        // in the result set.
        results<-&Result{documentUrl: item.url, response: response, urls: urls}

        // The first worker to hit maxDepth closes the queues, forcing all
        // workers to quit after their current processing and the main Crawl
        // goroutine to pop out of the result-gathering loop.
        if item.depth >= maxDepth {
            log.Print("reached max depth")
            close(queue)
            close(results)
            return
        }

        // Add the new urls we found to the queue.
        for _, newUrl := range urls {
            queue<-queueItem{url:newUrl, depth:item.depth+1}
            log.Print("added new url to queue: ", newUrl)
            runtime.Gosched()
        }

        //log.Print("worker finished: ", item.url)
    }

    return
}

func getUrls(resp *http.Response, parentUrlStr string) []string {
    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        //log.Print(err)
        return []string{}
    }
    resp.Body.Close()

    // We could have chosen to use a third-party library to extract urls here,
    // but since this is a "challenge", I opted for only using tools available
    // in the standard library. It does not yet have a stable html package to
    // help with this (it's still experimental :).
    // Regexps are perhaps not the best way to deal with this, but it'll have
    // to do for this quick demo.
    re := regexp.MustCompile("href=['\"]?([^'\" >]+)")

    urls := re.FindAllStringSubmatch(string(body), -1)

    if urls == nil {
        return []string{}
    }

    output := []string{}
    // Parent urls should always parse validly, so we throw away error in this
    // simple version.
    parentUrl, _ := url.Parse(parentUrlStr)
    for _, u := range urls {
        // Since the regexp is a bit shaky, we test the validity of the url
        // with url.Parse, and only relay valid urls.
        if up, err := parentUrl.Parse(u[1]); err == nil {
            // If up is absolute url, up is returned directly.
            output = append(output, up.String())
            //log.Print("url found: ", up.String())
        } else {
            //log.Print("url failed parsing: ", u[1])
        }
    }

    return output
}
