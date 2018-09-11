package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"splitwiseExpenseAPI/controller"

	"github.com/gorilla/mux"
)

var router = mux.NewRouter()
var Trace *log.Logger

func main() {

	//initialize logger
	logFilePathPtr := flag.String("log", "splitwiseAPIServer.log", "log file path - default splitwiseAPIServer.log will be used")

	//read config
	configFilePathPtr := flag.String("config", "splitwiseconfig.json", "config file path - default splitwiseconfig.json will be used")
	flag.Parse()

	//controller logger
	traceFile, _ := os.OpenFile(*logFilePathPtr, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0600)
	controller.InitLogger(traceFile)
	defer traceFile.Close()

	controller.InitializeConfig(*configFilePathPtr)

	//initialize router
	http.Handle("/", router)

	//add handlers
	router.HandleFunc("/", controller.IndexHandler)
	http.HandleFunc("/expenses", controller.CompleteAuth)
	http.HandleFunc("/getStoredJson", controller.GetStoredJson)
	http.HandleFunc("/getStoredJsonFile", controller.GetStoredJsonFile)

	//listen
	err := http.ListenAndServe(":9093", nil)
	if err != nil {
		Trace.Fatal("ListenAndServe", err)
	}
}
