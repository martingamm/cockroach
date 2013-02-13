package main

import (
    "fmt"
    "runtime"
)

func main() {
    // Set the crawler to use all available cores.
    runtime.GOMAXPROCS(runtime.NumCPU())
    // The seed url, max depth and number of worker-goroutines to be used.
    //c := Crawl("http://telenor.com", 10, 100)
    c := Crawl("http://vg.no", 10, 10000)

    fmt.Println(c)
}
