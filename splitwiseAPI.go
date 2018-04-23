package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"splitwiseExpenseAPi/controller"

	"github.com/gorilla/mux"
)

var router = mux.NewRouter()
var Trace *log.Logger

func main() {

	//initialize logger
	logFile, _ := os.OpenFile("log.txt", os.O_CREATE|os.O_APPEND|os.O_RDWR, 0600)
	defer logFile.Close()

	//controller logger
	controller.InitLogger(logFile)

	//read config
	configFilePathPtr := flag.String("config", "splitwiseconfig.json", "config file path - default splitwiseconfig.json will be used")
	controller.InitializeConfig(*configFilePathPtr)

	//initialize router
	http.Handle("/", router)

	//add handlers
	router.HandleFunc("/", controller.IndexHandler)
	http.HandleFunc("/expenses", controller.CompleteAuth)
	http.HandleFunc("/getStoredJson", controller.GetStoredJson)

	//listen
	err := http.ListenAndServe(":9093", nil)
	if err != nil {
		Trace.Fatal("ListenAndServe", err)
	}
}
