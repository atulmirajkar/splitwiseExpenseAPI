package controller

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"net/url"

	"github.com/dghubble/oauth1"
)

type Configuration struct {
	AccessTokenURL  string `json:"AccessTokenURL"`
	AuthorizeURL    string `json:"AuthorizeURL"`
	RequestTokenURL string `json:"RequestTokenURL"`
	ConsumerKey     string `json:"ConsumerKey"`
	ConsumerSecret  string `json:"ConsumerSecret"`
	CallbackURL     string `json: "CallbackURL"`
}

var Trace *log.Logger

var splitwiseEndPoint = new(oauth1.Endpoint)

var splitwiseAuthConfig = new(oauth1.Config)

var requestTok = ""
var requestSec = ""

var config = new(Configuration)
var ConfigFilePath string

//initialize config file
func InitializeConfig(filePath string) {
	//read json file
	file, err := ioutil.ReadFile(filePath)
	if err != nil {
		fmt.Println("error reading config file - Exiting")
		os.Exit(1)
	}

	//marshall configuration object
	err = json.Unmarshal(file, config)
	if err != nil {
		fmt.Println("error reading config file - Exiting", err)
		os.Exit(1)
	}

	splitwiseEndPoint = &oauth1.Endpoint{
		AccessTokenURL:  config.AccessTokenURL,
		AuthorizeURL:    config.AuthorizeURL,
		RequestTokenURL: config.RequestTokenURL,
	}

	splitwiseAuthConfig = &oauth1.Config{
		ConsumerKey:    config.ConsumerKey,
		ConsumerSecret: config.ConsumerSecret,
		CallbackURL:    config.CallbackURL,
		Endpoint:       *splitwiseEndPoint,
	}
}

func InitLogger(file *os.File) {
	if file != nil {
		Trace = log.New(file,
			"TRACE: ",
			log.Ldate|log.Ltime|log.Lshortfile)
	}
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {

	Trace.Println("Got request for:", r.URL.String())

	//1. Your application requests authorization
	requestToken, requestSecret, err := splitwiseAuthConfig.RequestToken()
	requestTok = requestToken
	requestSec = requestSecret
	if err != nil {
		Trace.Fatal(err)
	}
	authorizationURL, err := splitwiseAuthConfig.AuthorizationURL(requestToken)
	if err != nil {
		Trace.Fatal(err)
	}
	http.Redirect(w, r, authorizationURL.String(), http.StatusFound)
}

func CompleteAuth(w http.ResponseWriter, r *http.Request) {
	// use the token to get an authenticated client
	requestTok, verifier, err := oauth1.ParseAuthorizationCallback(r)
	if err != nil {
		Trace.Fatal(err)
	}
	accessToken, accessSecret, err := splitwiseAuthConfig.AccessToken(requestTok, requestSec, verifier)
	if err != nil {
		Trace.Fatal(err)
	}
	token := oauth1.NewToken(accessToken, accessSecret)

	// httpClient will automatically authorize http.Request's
	httpClient := splitwiseAuthConfig.Client(oauth1.NoContext, token)
	response, err := httpClient.Get("https://secure.splitwise.com/api/v3.0/get_groups")

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)

	//fmt.Fprintf(w, "Content: %s\n", contents)

	var groupData interface{}
	err = json.Unmarshal(contents, &groupData)
	if err != nil {
		Trace.Fatal(err)
	}

	_, err = os.Stat("expenses.csv")
	if !os.IsNotExist(err) {
		err := os.Remove("expenses.csv")
		if err != nil {
			Trace.Fatalln(err)
		}

	}

	f, err := os.OpenFile("expenses.csv", os.O_CREATE, 0755)
	if err != nil {
		Trace.Fatalln(err)
	}
	//header
	if _, err = f.WriteString("Group,Date,Description,Category,Cost,User,Share,\n"); err != nil {
		Trace.Fatal(err)
	}

	groupDatArr := groupData.(map[string]interface{})["groups"].([]interface{})
	for _, group := range groupDatArr {
		groupMap := group.(map[string]interface{})
		//fmt.Fprintf(w, "%s \n", groupMap["name"])
		requestURL := GetURLForGroup(groupMap["id"].(float64))
		expenseResponse, _ := httpClient.Get(requestURL)
		contents, _ := ioutil.ReadAll(expenseResponse.Body)
		//fmt.Fprintf(w, "%s\n", contents)

		defer f.Close()

		expenseLine := writeExpenseDataForGroup(contents, groupMap["name"].(string), f)
		fmt.Fprintf(w, "%s\n", expenseLine)

	}
}

func GetURLForGroup(groupID float64) string {
	requestURL, _ := url.Parse("https://secure.splitwise.com/api/v3.0/get_expenses")
	requestQuery := requestURL.Query()
	requestQuery.Set("group_id", strconv.FormatFloat(groupID, 'f', 0, 64))
	requestQuery.Set("limit", "0")
	requestURL.RawQuery = requestQuery.Encode()
	return requestURL.String()
}

func writeExpenseDataForGroup(expensesData []byte, groupName string, file *os.File) string {
	var expenseLine = ""
	var unMarsharlData interface{}
	err := json.Unmarshal(expensesData, &unMarsharlData)
	if err != nil {
		Trace.Fatal(err)
	}
	expensesDataArr := unMarsharlData.(map[string]interface{})["expenses"].([]interface{})
	for _, expense := range expensesDataArr {
		expenseMap := expense.(map[string]interface{})
		//extract info
		date := expenseMap["date"].(string)
		description := expenseMap["description"].(string)
		description = strings.Replace(description, ",", "", -1)
		category := expenseMap["category"].(map[string]interface{})
		categoryStr := category["name"].(string)
		categoryStr = strings.Replace(categoryStr, ",", "", -1)
		cost := expenseMap["cost"].(string)
		cost = strings.Replace(cost, ",", "", -1)
		userArr := expenseMap["users"].([]interface{})
		user := getUserInfo(userArr)
		userStrArr := strings.Split(user, ",")
		for _, userNameOwedStr := range userStrArr {
			tempUserNameOwedArr := strings.Split(userNameOwedStr, "_")
			tempStrArr := []string{groupName, date, description, categoryStr, cost, tempUserNameOwedArr[0], tempUserNameOwedArr[1], "\n"}
			tempStr := strings.Join(tempStrArr, ",")
			if expenseLine == "" {
				expenseLine = tempStr
			} else {
				expenseLineArr := []string{expenseLine, tempStr}
				expenseLine = strings.Join(expenseLineArr, ",")
			}
			if _, err = file.WriteString(tempStr); err != nil {
				Trace.Fatal(err)
			}
		}

	}
	return expenseLine
}

func getUserInfo(userArr []interface{}) string {
	var userLine = ""
	for _, user := range userArr {
		userMap := user.(map[string]interface{})
		userInfoMap := userMap["user"].(map[string]interface{})
		userName := userInfoMap["first_name"].(string)
		userName = strings.Replace(userName, ",", "", -1)
		userShare := ""
		if userMap["owed_share"] != nil {
			userShare = userMap["owed_share"].(string)
			userShare = strings.Replace(userShare, ",", "", -1)

		}

		tempStrArr := []string{userName, userShare}
		tempStr := strings.Join(tempStrArr, "_")
		if userLine == "" {
			userLine = tempStr
		} else {
			userLineArr := []string{userLine, tempStr}
			userLine = strings.Join(userLineArr, ",")
		}

	}
	return userLine
}
