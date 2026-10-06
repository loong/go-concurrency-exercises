package main

import (
	"context"
	"errors"
	"fmt"
	"go_concurrency/cancellation/imp"
	"time"
)

func main() {
	// Initialize your DB with the mock driver
	db := imp.NewYourDB(imp.NewMockEmulatedDriver())

	// --- Test Case 1: Timeout (Query takes longer than context) ---
	fmt.Println("\n--- Test Case 1: Timeout ---")
	ctx1, cancel1 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel1()

	start := time.Now()
	rows1, err1 := db.QueryContext(ctx1, "SLOW QUERY")
	duration := time.Since(start)
	fmt.Printf("Query 1 completed in %v\n", duration)

	if err1 != nil {
		if errors.Is(err1, context.DeadlineExceeded) {
			fmt.Printf("Query 1 result: Expected error (Context deadline exceeded).\n")
		} else {
			fmt.Printf("Query 1 result: Unexpected error: %v\n", err1)
		}
	} else {
		defer rows1.Close()
		var data string
		for rows1.Next() {
			rows1.Scan(&data)
			fmt.Printf("Data: %s\n", data)
		}
		fmt.Println("Query 1 result: Succeeded (unexpected).")
	}

	// --- Test Case 2: Explicit Cancellation (Query is canceled before completion) ---
	fmt.Println("\n--- Test Case 2: Explicit Cancellation ---")
	ctx2, cancel2 := context.WithCancel(context.Background())

	go func() {
		time.Sleep(1 * time.Second) // Cancel after 1 second
		fmt.Println("Main: Calling cancel2() for Query 2.")
		cancel2()
	}()

	start = time.Now()
	rows2, err2 := db.QueryContext(ctx2, "ANOTHER SLOW QUERY")
	duration = time.Since(start)
	fmt.Printf("Query 2 completed in %v\n", duration)

	if err2 != nil {
		if errors.Is(err2, context.Canceled) {
			fmt.Printf("Query 2 result: Expected error (Context canceled).\n")
		} else {
			fmt.Printf("Query 2 result: Unexpected error: %v\n", err2)
		}
	} else {
		defer rows2.Close()
		var data string
		for rows2.Next() {
			rows2.Scan(&data)
			fmt.Printf("Data: %s\n", data)
		}
		fmt.Println("Query 2 result: Succeeded (unexpected).")
	}

	// --- Test Case 3: Query Completes Successfully (within context) ---
	fmt.Println("\n--- Test Case 3: Success ---")
	ctx3, cancel3 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel3()

	start = time.Now()
	rows3, err3 := db.QueryContext(ctx3, "FAST QUERY") // This query is designed to be faster
	duration = time.Since(start)
	fmt.Printf("Query 3 completed in %v\n", duration)

	if err3 != nil {
		fmt.Printf("Query 3 result: Error: %v\n", err3)
	} else {
		defer rows3.Close()
		var data string
		found := false
		for rows3.Next() {
			rows3.Scan(&data)
			fmt.Printf("Query 3 Data: %s\n", data)
			found = true
		}
		if !found {
			fmt.Println("Query 3 result: No rows found.")
		}
		fmt.Println("Query 3 result: Succeeded (expected).")
	}

	// Give time for logs to print
	time.Sleep(100 * time.Millisecond)
}
