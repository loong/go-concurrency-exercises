//////////////////////////////////////////////////////////////////////
//
// DO NOT EDIT THIS PART
// Your task is to edit `main.go`
//

package main

import "fmt"

// MockFetcher is Fetcher that returns canned results. Taken from
// https://tour.golang.org/concurrency/10
type MockFetcher map[string]*mockResult

type mockResult struct {
	body string
	urls []string
}

// Fetch pretends to retrieve the URLs and its subpages
func (f MockFetcher) Fetch(url string) (string, []string, error) {
	fetchSignalInstance() <- true
	if res, ok := f[url]; ok {
		return res.body, res.urls, nil
	}
	return "", nil, fmt.Errorf("not found: %s", url)
}

// fetcher is a populated MockFetcher.
var fetcher = MockFetcher{
	"http://golang.org/": &mockResult{
		"The Go Programming Language",
		[]string{
			"http://golang.org/pkg/",
			"http://golang.org/cmd/",
		},
	},
	"http://golang.org/pkg/": &mockResult{
		"Packages",
		[]string{
			"http://golang.org/",
			"http://golang.org/cmd/",
			"http://golang.org/pkg/fmt/",
			"http://golang.org/pkg/os/",
		},
	},
	"http://golang.org/pkg/fmt/": &mockResult{
		"Package fmt",
		[]string{
			"http://golang.org/",
			"http://golang.org/pkg/",
		},
	},
	"http://golang.org/pkg/os/": &mockResult{
		"Package os",
		[]string{
			"http://golang.org/",
			"http://golang.org/pkg/",
		},
	},
}

//////////////////////////////////////////////////////////////////////
// Code below is mainly used to test whether a solution is correct or
// not

// fetchSignal is used to test whether the solution is correct
var fetchSignal chan bool

// fetchSignalInstance is a singleton to access fetchSignal
func fetchSignalInstance() chan bool {
	if fetchSignal == nil {
		// Use buffered channel to avoid blocking
		fetchSignal = make(chan bool, 1000)
	}
	return fetchSignal
}
