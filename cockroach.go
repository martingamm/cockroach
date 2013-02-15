// cockroach.go
// A very simple webcrawler written in Go.
// Made by Martin Gammelsæter (martingammelsaeter@gmail.com)
//
// (Excessive commenting on request!)
package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

const (
	// Number of workers chosen by some rather unscientific tests of different
	// values.
	N_WORKERS = 1000
)

type result struct {
	// We store http.Responses instead of the body string or similar because
	// http.Results are standardized and easy to work with in Go, if this
	// crawler was to be used as a library by some other application.
	response *http.Response
	urls     []string
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
	// By using a map from string to result we achieve a tree-like structure in
	// a one-dimensional map. For every url, we can see it's "children" urls
	// and look them up in the original map and so on to see the
	// tree-structure.
	results map[string]*result
	// All the dispatchers including the main crawl goroutine will be accessing
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
	// number of goroutines is kept. In all honesty, we could possibly have a
	// situation of a growing amount of dispatchers if they took longer to
	// complete their task than a worker. This is however unlikely since the
	// operation that takes clearly the longest time here is the http.Get().
	// We could mitigate this if it should prove a problem by not spinning up
	// detachers as goroutines, but do them blocking instead.
	// The number of workers should be chosen so as to maximize throughput
	// while minimizing memory-utilization, and we would need some epirical
	// testing to figure out the optimum for a given system.
	for w := 1; w <= N_WORKERS; w++ {
		go worker(urlChan, resultChan, output)
	}

	// This channel will never really be closed, and if our main program was
	// not terminating after having received the output (and printing it) we'd
	// just keep crawling for ever. Since we have many dispatchers, we can't
	// outright close the channel; we'd get other dispatchers then trying to
	// send on a closed channel (blows up).
	// In this simple example we don't have to close anything since the main
	// thread returns once the results are in, and we avoid some complexity in
	// messaging all goroutines to stop what they are doing, which would
	// probably be quite messy since we have an unknown amount of dispatchers.
	urlChan <- urlChanItem{url: seedUrl, depth: 0}

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
		results <- resultChanItem{
			url:    item.url,
			result: &result{response: response, urls: urls},
			depth:  item.depth}

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

		queue <- urlChanItem{url: newUrl, depth: depth + 1}
		log.Print("added new url to queue: ", newUrl)
	}
}

func getUrls(resp *http.Response, parentUrlStr string) []string {
	output := []string{}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return output
	}

	// We could have chosen to use a third-party library to extract urls here,
	// but since this is a "challenge", I opted for only using tools available
	// in the standard library. It does not yet have a stable html package to
	// help with this (it's still experimental :).  Regexps are perhaps not the
	// best way to deal with this, but it'll have to do for this quick demo.
	re := regexp.MustCompile("href=['\"]?([^'\" >]+)")

	urls := re.FindAllStringSubmatch(string(body), -1)
	if urls == nil {
		return output
	}

	// Parent urls should always parse validly, so we throw away errors in this
	// simple version.
	parentUrl, _ := url.Parse(parentUrlStr)

	for _, u := range urls {
		// Since the regexp is a bit shaky, we test the validity of the url
		// with url.Parse, and only relay valid urls.
		if up, err := parentUrl.Parse(u[1]); err == nil {
			uo := up.String()
			// Pruning some simple things to clean the dataset. There are
			// probably more things that could be excluded, but these were the
			// most prevalent false positives.
			if strings.HasPrefix(uo, "mailto:") {
				continue
			}
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

var seedUrl = flag.String("s", "", "seed URL.")
var maxDepth = flag.Int("d", 10, "maximum depth to crawl.")

func main() {
	// No gain in letting this run on multiple cores, as it is not cpu-bound.
	// Using more cores simply slows the program down due to overhead in
	// channel communication.
	runtime.GOMAXPROCS(1)

	flag.Parse()
	if *seedUrl == "" {
		fmt.Fprintln(os.Stderr, "Seed URL cannot be empty. See "+os.Args[0]+" --help.")
		os.Exit(1)
	}

	// Letting crawl have a return value was done for simplicity, but one could
	// choose to make a channel through which one could receive results as they
	// came in if one wanted to process the results concurrently.
	c := crawl(*seedUrl, *maxDepth)

	fmt.Println("URLs crawled:")
	fmt.Println("=============")
	i := 0
	// The contents and "children" of a URL is not used, as this is simply to
	// show that it works :)
	for url, _ := range c {
		fmt.Println(url)
		i++
	}

	fmt.Println("Total URLs crawled: ", i)
}
