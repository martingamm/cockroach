package main

import (
    "fmt"
    "runtime"
)

func main() {
    // Set the crawler to use all available cores.
    runtime.GOMAXPROCS(runtime.NumCPU())
    //runtime.GOMAXPROCS(1)
    // The seed url, max depth and number of worker-goroutines to be used.
    c := crawl("http://telenor.no", 10)

    fmt.Println("URLs crawled:")
    fmt.Println("=============")
    i := 0
    for k, _ := range c {
        fmt.Println(k)
        i++
    }

    fmt.Println("Total: ", i)
}
