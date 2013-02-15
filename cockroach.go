// TODO: Add quit after x seconds if depth not reached.
// TODO: Stalls if < 10000 goroutines for some reason.
package main

import (
    "net/http"
    "net/url"
    "sync"
    "log"
    "io/ioutil"
    "regexp"
    "strings"
)

const (
    N_WORKERS = 10000
)

type result struct {
    // We store http.Responses instead of the body string or similar because
    // http.Results are standardized and easy to work with in Go, if this
    // crawler was to be used as a library by some other application.
    response    *http.Response
    urls        []string
}

type urlChanItem struct {
    url   string
    depth int
}

type resultChanItem struct {
    url    string
    result *result
    depth  int
}

type outputmap struct {
    // For this minimal case we'll just keep all results in memory with the
    // results-map, but in a real-world scenario we'd want to dump results to
    // disk at least every now and then. Without dumping to disk the memory
    // consumption of the crawler will steadily rise unchecked.
    results map[string]*result
    // All the goroutines including the main Crawl goroutine will be accessing
    // the result-map, so we need a way to lock it.
    sync.Mutex
}

func crawl(seedUrl string, maxDepth int) map[string]*result {
    output := outputmap{results: make(map[string]*result)}
    urlChan := make(chan urlChanItem, 100) // Rather arbitrary buffer-sizes.
    resultChan := make(chan resultChanItem, 100)

    // Spinning up a number of worker-goroutines. We could alternatively choose
    // to spin up a new goroutine for every new url discovered, but we would
    // risk running out of memory on some systems, so a pool of a manageable
    // number of goroutines is kept. The number should be chosen so as to
    // maximize throughput while minimizing memory-utilization, and we would
    // need some epirical testing to figure out the optimum for a given system.
    for w:= 1; w <= N_WORKERS; w++ {
        go worker(urlChan, resultChan, output)
    }

    // This channel will never really be closed, and if our main program was
    // not terminating after having received the output (and printing it) we'd
    // just keep crawling for ever. Since we have many dispatchers, we can't
    // outright close the channel; we'd get other dispatchers then trying to
    // send on a closed channel (blows up).
    // If the main function was supposed to keep going, we'd have to have FIXME
    urlChan<-urlChanItem{url:seedUrl, depth: 0}

    for res := range resultChan {
        output.Lock()
        output.results[res.url] = res.result
        output.Unlock()

        if res.depth >= maxDepth {
            log.Print("reached max depth")
            break
        }
    }

    // maps in Go are a reference-type, so no use returning a pointer.
    return output.results
}

func worker(queue chan urlChanItem, results chan resultChanItem, output outputmap) {
    for item := range queue {
        log.Print("worker began on url: ", item.url, ", at depth: ", item.depth)

        response, err := http.Get(item.url)
        // If something wrong happened to the request, log what went wrong and go
        // to next url in queue. No use in panicking here, since we can still
        // get bad links (they might return 5xx-codes for example).
        if err != nil {
            log.Print(err)
            continue
        }

        urls := getUrls(response, item.url)
        response.Body.Close()

        // Send the result to main goroutine.
        results<-resultChanItem{
          url: item.url,
          result: &result{response: response, urls: urls},
          depth: item.depth}

        // Add the new urls we found to the queue through a dispatcher.
        go dispatcher(urls, output, queue, item.depth)
    }
}

func dispatcher(urls []string, output outputmap, queue chan urlChanItem, depth int) {
    for _, newUrl := range urls {
        output.Lock()
        // If the url is in the result-set already, we skip ahead.
        if _, ok := output.results[newUrl]; ok {
            output.Unlock()
            continue
        }
        // Put placeholder so other workers correctly skip.
        output.results[newUrl] = &result{}
        output.Unlock()

        queue<-urlChanItem{url: newUrl, depth: depth+1}
        log.Print("added new url to queue: ", newUrl)
    }
}

func getUrls(resp *http.Response, parentUrlStr string) []string {
    body, err := ioutil.ReadAll(resp.Body)
    if err != nil {
        return []string{}
    }

    // We could have chosen to use a third-party library to extract urls here,
    // but since this is a "challenge", I opted for only using tools available
    // in the standard library. It does not yet have a stable html package to
    // help with this (it's still experimental :).  Regexps are perhaps not the
    // best way to deal with this, but it'll have to do for this quick demo.
    re := regexp.MustCompile("href=['\"]?([^'\" >]+)")

    urls := re.FindAllStringSubmatch(string(body), -1)
    if urls == nil {
        return []string{}
    }

    // Parent urls should always parse validly, so we throw away errors in this
    // simple version.
    parentUrl, _ := url.Parse(parentUrlStr)

    output := []string{}
    for _, u := range urls {
        // Since the regexp is a bit shaky, we test the validity of the url
        // with url.Parse, and only relay valid urls.
        if up, err := parentUrl.Parse(u[1]); err == nil {
            uo := up.String()
            // Pruning away everything after a #, it isn't in the http-protocol
            // anyways. This way we avoid having multiples of what is
            // essentially the same page in the result set.
            if i := strings.Index(uo, "#"); i > -1 {
                uo = uo[:i]
            }
            output = append(output, uo)
            log.Print("url found: ", uo)
        }
    }

    return output
}
