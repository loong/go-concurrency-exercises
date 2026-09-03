# Go Concurrency Exercises [![Build Status](https://travis-ci.org/loong/go-concurrency-exercises.svg?branch=main)](https://travis-ci.org/loong/go-concurrency-exercises) [![Go Report Card](https://goreportcard.com/badge/github.com/loong/go-concurrency-exercises)](https://goreportcard.com/report/github.com/loong/go-concurrency-exercises)
Exercises for Golang's concurrency patterns.

## Why
The Go community has plenty resources to read about go's concurrency model and how to use it effectively. But *who actually wants to read all this*!? This repo tries to teach concurrency patterns by following the 'learning by doing' approach.

![Image of excited gopher](https://golang.org/doc/gopher/pkg.png)

## How to take this challenge
1. *Only edit `main.go`* to solve the problem. Do not touch any of the other files.
2. If you find a `*_test.go` file, you can test the correctness of your solution with `go test`
3. If you get stuck, join us on [Discord](https://discord.com/invite/golang) or [Slack](https://invite.slack.golangbridge.org/)! Surely there are people who are happy to give you some code reviews (if not, find me via `@loong` ;) )

## For coding agents

This workshop is set up for AI coding agents as well as humans:

- [`AGENTS.md`](./AGENTS.md) — how to coach a learner, which files are frozen, and how to run tests ([AGENTS.md](https://agents.md/) convention)
- [`llms.txt`](./llms.txt) — curated index for crawlers and chat agents ([llms.txt](https://llmstxt.org/) convention)
- [`exercises.json`](./exercises.json) — machine-readable catalog of every challenge
- [`.agents/skills/`](./.agents/skills/) — on-demand skills (`take-exercise`, `maintain-exercises`)

Each exercise directory also has its own `AGENTS.md`. Please do not paste full solutions into `main.go` unless the learner asks.

## Overview
| # | Name of the Challenge + URL |
| - |:-------------|
| 0 | [Limit your Crawler](https://github.com/loong/go-concurrency-exercises/tree/main/0-limit-crawler) |
| 1 | [Producer-Consumer](https://github.com/loong/go-concurrency-exercises/tree/main/1-producer-consumer) |
| 2 | [Race Condition in Caching Cache](https://github.com/loong/go-concurrency-exercises/tree/main/2-race-in-cache#race-condition-in-caching-szenario) |
| 3 | [Limit Service Time for Free-tier Users](https://github.com/loong/go-concurrency-exercises/tree/main/3-limit-service-time) |
| 4 | [Graceful SIGINT Killing](https://github.com/loong/go-concurrency-exercises/tree/main/4-graceful-sigint) |
| 5 | [Clean Inactive Sessions to Prevent Memory Overflow](https://github.com/loong/go-concurrency-exercises/tree/main/5-session-cleaner) |

## License

```
 DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE 
                    Version 2, December 2004 

 Copyleft from 2017 Long Hoang

 Everyone is permitted to copy and distribute verbatim or modified 
 copies of this license document, and changing it is allowed as long 
 as the name is changed.

            DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE 
   TERMS AND CONDITIONS FOR COPYING, DISTRIBUTION AND MODIFICATION 

  0. You just DO WHAT THE FUCK YOU WANT TO.
```
