package controller

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dghubble/oauth1"
	"github.com/gorilla/securecookie"
	"github.com/pkg/errors"
)

type sessionValues struct {
	sessionID string
	token     *oauth1.Token
}
type Configuration struct {
	AccessTokenURL  string `json:"AccessTokenURL"`
	AuthorizeURL    string `json:"AuthorizeURL"`
	RequestTokenURL string `json:"RequestTokenURL"`
	ConsumerKey     string `json:"ConsumerKey"`
	ConsumerSecret  string `json:"ConsumerSecret"`
	CallbackURL     string `json: "CallbackURL"`
	DataPath        string `json: "DataPath"`
	ShinyPort       string `json: "ShinyPort"`
}

var Trace *log.Logger

var splitwiseEndPoint = new(oauth1.Endpoint)

var splitwiseAuthConfig = new(oauth1.Config)

var requestTok = ""
var requestSec = ""

var cookieHandler = securecookie.New(securecookie.GenerateRandomKey(64), securecookie.GenerateRandomKey(32))
var sessionMapper = make(map[string]*sessionValues)

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

	sessionToken := oauth1.NewToken(accessToken, accessSecret)
	user := getCurrentUserID(sessionToken)

	sessionID := createSessionID()
	value := map[string]string{
		"username":  user,
		"sessionid": sessionID,
	}
	encoded, err := cookieHandler.Encode("clientMap", value)
	if err != nil {
		Trace.Println("error encoding cookie")
		errors.Wrap(err, "error encoding cookie")
	}
	cookie := &http.Cookie{
		Name:   "clientMap",
		Value:  encoded,
		Path:   "/",
		MaxAge: 300, //5 minutes
	}
	http.SetCookie(w, cookie)

	//save session in a map
	sessionMapper[user] = &sessionValues{sessionID: sessionID, token: sessionToken}
	http.Redirect(w, r, "http://localhost:"+config.ShinyPort+"?file="+user, http.StatusFound)
}

func createSessionID() string {
	//return a random string
	timeNow := time.Now()
	randSource := timeNow.Hour()*3600 + timeNow.Minute()*60 + timeNow.Second()
	r := rand.New(rand.NewSource(int64(randSource)))
	return strconv.Itoa(r.Intn(100000))
}

func getCurrentUserID(sessionToken *oauth1.Token) string {
	// httpClient will automatically authorize http.Request's
	httpClient := splitwiseAuthConfig.Client(oauth1.NoContext, sessionToken)
	response, err := httpClient.Get("https://secure.splitwise.com/api/v3.0/get_current_user")

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)

	var userData interface{}
	err = json.Unmarshal(contents, &userData)
	if err != nil {
		Trace.Fatal(err)
	}

	userDataObj := userData.(map[string]interface{})
	userDataMap := userDataObj["user"].(map[string]interface{})

	return strconv.FormatFloat(userDataMap["id"].(float64), 'f', 0, 64)

}
func getUserFilePath(user string) (string, error) {
	if user == "" {
		return "", errors.New("user file path is empty")
	}
	return config.DataPath + "\\" + user + ".csv", nil
}

func GetStoredJsonFile(w http.ResponseWriter, r *http.Request) {
	user := r.URL.Query().Get("file")

	//check file creation/modification time
	// get last modified time
	fileName, err := getUserFilePath(user)
	if err != nil {
		Trace.Println(err)
		return
	}
	fileInfo, err := os.Stat(fileName)

	if err != nil && os.IsNotExist(err) {
		saveExpenseDataToCSV(sessionMapper[user].token)
		fileInfo, err = os.Stat(fileName)
	}
	if err != nil {
		Trace.Println(err)
		return
	}

	modifiedtime := fileInfo.ModTime()
	currentTime := time.Now()

	timeDiff := currentTime.Sub(modifiedtime)

	if timeDiff.Seconds() > 500 {
		saveExpenseDataToCSV(sessionMapper[user].token)
	}
	//read file
	csvFile, err := os.Open(fileName)
	if err != nil {
		Trace.Println(err)
		return
	}
	defer csvFile.Close()

	//csv reader
	csvReader := csv.NewReader(bufio.NewReader(csvFile))

	if err != nil {
		Trace.Println(err)
		return
	}

	var expenseLine ExpenseLine
	var expenses []ExpenseLine

	for {
		each, error := csvReader.Read()
		if error == io.EOF {
			break
		} else if error != nil {
			Trace.Println(error)
			break
		}

		expenseLine.Group = each[0]
		expenseLine.Date = each[1]
		expenseLine.Description = each[2]
		expenseLine.Category = each[3]
		expenseLine.Cost = each[4]
		expenseLine.User = each[5]
		expenseLine.Share = each[6]

		//add to expenses object
		expenses = append(expenses, expenseLine)
	}

	// Convert to JSON
	jsonData, err := json.Marshal(expenses)
	if err != nil {
		Trace.Println(err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

func GetStoredJson(w http.ResponseWriter, r *http.Request) {
	sessionVals := validateSessionAndGetUser(r)
	if sessionVals == nil {
		return
	}

	user := getCurrentUserID(sessionVals.token)
	//check file creation/modification time
	// get last modified time
	fileName, err := getUserFilePath(user)
	if err != nil {
		Trace.Println(err)
		return
	}
	fileInfo, err := os.Stat(fileName)

	if err != nil && os.IsNotExist(err) {
		saveExpenseDataToCSV(sessionVals.token)
		fileInfo, err = os.Stat(fileName)
	}
	if err != nil {
		Trace.Println(err)
		return
	}

	modifiedtime := fileInfo.ModTime()
	currentTime := time.Now()

	timeDiff := currentTime.Sub(modifiedtime)

	if timeDiff.Seconds() > 500 {
		saveExpenseDataToCSV(sessionVals.token)
	}
	//read file
	csvFile, err := os.Open(fileName)
	if err != nil {
		Trace.Println(err)
		return
	}
	defer csvFile.Close()

	//csv reader
	csvReader := csv.NewReader(bufio.NewReader(csvFile))

	if err != nil {
		Trace.Println(err)
		return
	}

	var expenseLine ExpenseLine
	var expenses []ExpenseLine

	for {
		each, error := csvReader.Read()
		if error == io.EOF {
			break
		} else if error != nil {
			Trace.Println(error)
			break
		}

		expenseLine.Group = each[0]
		expenseLine.Date = each[1]
		expenseLine.Description = each[2]
		expenseLine.Category = each[3]
		expenseLine.Cost = each[4]
		expenseLine.User = each[5]
		expenseLine.Share = each[6]

		//add to expenses object
		expenses = append(expenses, expenseLine)
	}

	// Convert to JSON
	jsonData, err := json.Marshal(expenses)
	if err != nil {
		Trace.Println(err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonData)
}

func validateSessionAndGetUser(request *http.Request) *sessionValues {
	var cookieUserName string
	var cookieSession string
	var storedSession string

	//get cookie from client request
	cookie, err := request.Cookie("clientMap")
	if err != nil {
		Trace.Println(err)
		return nil

	}
	cookieValue := make(map[string]string)
	err = cookieHandler.Decode("clientMap", cookie.Value, &cookieValue)
	if err != nil {
		Trace.Println(err)
		return nil
	}

	cookieUserName = cookieValue["username"]
	cookieSession = cookieValue["sessionid"]
	storedSession = sessionMapper[cookieUserName].sessionID
	//compare session ids
	if cookieSession != storedSession {
		return nil
	}

	//get stored sessionid from server
	return sessionMapper[cookieUserName]

}

func saveExpenseDataToCSV(sessionToken *oauth1.Token) {
	// httpClient will automatically authorize http.Request's
	httpClient := splitwiseAuthConfig.Client(oauth1.NoContext, sessionToken)
	response, err := httpClient.Get("https://secure.splitwise.com/api/v3.0/get_groups")

	defer response.Body.Close()
	contents, err := ioutil.ReadAll(response.Body)

	var groupData interface{}
	err = json.Unmarshal(contents, &groupData)
	if err != nil {
		Trace.Fatal(err)
	}

	user := getCurrentUserID(sessionToken)
	//check file creation/modification time
	// get last modified time
	fileName, err := getUserFilePath(user)
	if err != nil {
		Trace.Println(err)
		return
	}
	_, err = os.Stat(fileName)
	if err != nil {
		err := os.Remove(fileName)
		if err != nil {
			Trace.Println(err)
		}

	}

	f, err := os.Create(fileName)
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
		requestURL := GetURLForGroup(groupMap["id"].(float64))
		expenseResponse, _ := httpClient.Get(requestURL)
		contents, _ := ioutil.ReadAll(expenseResponse.Body)

		defer f.Close()

		writeExpenseDataForGroup(contents, groupMap["name"].(string), f)
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
	//replace quotes in string
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

type ExpenseLine struct {
	Group       string
	Date        string
	Description string
	Category    string
	Cost        string
	User        string
	Share       string
}
