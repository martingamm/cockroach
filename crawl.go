package main

import (
    "fmt"
)

func main() {
    c := Crawl("http://telenor.com", 10)

    fmt.Println(c)
}
