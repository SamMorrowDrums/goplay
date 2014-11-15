package test

import "log"

func SayHi(name string) (greeting string) {
	greeting = "Hello " + name
	log.Print(greeting)
	return
}
