# Clean Inactive Sessions to Prevent Memory Overflow

Given is a SessionManager that stores session information in
memory. The SessionManager itself is working, however, since we
keep on adding new sessions to the manager our program will
eventually run out of memory.

Your task is to implement a session cleaner routine that runs
concurrently in the background and cleans every session that
hasn't been updated for more than 5 seconds (of course usually
session times are much longer).

Note that we expect the session to be removed anytime between 5 and 7
seconds after the last update. Also, note that you have to be very
careful in order to prevent race conditions.

## Test your solution

To complete this exercise, you must pass two test. The normal `go
test` test cases as well as the race condition test.

Use the following commands to test your solution:
```
go test
go test --race
```