///////////////////////////////////////////////////////////////////////
// Name:		Jenkins deployment lambda
// Repository:	All Repositories
// Target:		Jenkins
// Jobs:		All Production Jobs in jenkins
// Workflow: 	GITHUB-Tag -> ALB -> Lambda -> Jenkins+tag
// Description:	Deploying of jenkins job while parsing tags from their repsective repositories,
//				The deployment is locked down to DevOps team by checking if the user is listed in DevOps
///////////////////////////////////////////////////////////////////////
package main

import (
	"bytes"
	"context" 
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/iamtito/go/bolatito"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/google/go-github/github"
)

//  String variables
var (
	ReleaseAction string
	ReleaseTag    string
	ReleaseTarget string
	ReleaseUser   string
	ReleaseRepo   string
	channelID     string
	alertMessage  string
	slackToken    map[string]string
	jenkinsToken  map[string]string
	aws           shared.AwsInterface
)

// HealthCheck is a function which responds to alb health check request
func HealthCheck() (events.ALBTargetGroupResponse, error) {
	log.Println("Status: 200 OK")
	return events.ALBTargetGroupResponse{
		Headers:           map[string]string{"content-type": "application/json"},
		StatusCode:        200,
		StatusDescription: "200 OK",
		IsBase64Encoded:   true,
		Body:              "OK",
	}, nil
}

// SlackSetup - Obtain the map of Slack tokens from Secrets manager and return
func SlackSetup() map[string]string {
	// Initialize Slack API with the Bot Token
	channelID = os.Getenv("SLACK_CHANNEL")
	slackSecretName := os.Getenv("SLACK_TOKEN_SECRETNAME")
	aws = shared.ConstructAWS()
	token, err := aws.GetSecret(slackSecretName)
	if err != nil {
		log.Panicln("Couldn't get slack token - error:", err)
	}
	return token
}

// JenkinsSetup - Obtain the jenkins credential from secret manager
func JenkinsSetup() map[string]string {
	// Initialize to get Jenkins API Cred
	jenkinsAPI := os.Getenv("JENKINS_AUTH")
	aws = shared.ConstructAWS()
	jenkinsCred, err := aws.GetSecret(jenkinsAPI)
	if err != nil {
		log.Panicln("Couldn't get jenkins cred - error:", err)
	}
	return jenkinsCred
}

// DeployToJenkins handles triggering of jenkins' deployment
func DeployToJenkins(job string) (int, error) {
	var bytePayload []byte
	req, _ := http.NewRequest("POST", job, bytes.NewBuffer(bytePayload))

	req.Header.Add("Content-type", "application/json")

	client := http.Client{Timeout: 20 * time.Second}

	res, err := client.Do(req)
	if err != nil {
		msg := fmt.Sprintf("Post to jenkins failed: %+v", err)
		log.Print("Error! ", msg)
	}
	defer res.Body.Close()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		log.Print("Status code error posting to Jenkins! ", res.StatusCode)
		err = errors.New(res.Status)
		return res.StatusCode, err
	}
	log.Print("Successfully Deployed to Jenkins!")

	return res.StatusCode, nil
}

// handleWebhook handles incoming request from the application load balancer
func handleWebhook(ctx context.Context, albEvent events.ALBTargetGroupRequest) (events.ALBTargetGroupResponse, error) {
	log.Print("This request is a: ", albEvent.HTTPMethod, " request")
	if albEvent.HTTPMethod == "GET" {
		return HealthCheck()
	}
	log.Println("****** Starting... ***********")
	t := strings.NewReader(albEvent.Body)
	payload, err := ioutil.ReadAll(t)

	if err != nil {
		log.Printf("error reading request body: err=%s\n", err)
	}

	event, err := github.ParseWebHook(albEvent.Headers["x-github-event"], payload)
	if err != nil {
		log.Printf("could not parse webhook: err=%s\n", err)
	}
	//////////////////////////////////////////////////////////////////////////////////////////////////////
	///////////////// Turn On If The Payload Is Configure To Use Secret //////////////////////////////////
	//////////////////////////////////////////////////////////////////////////////////////////////////////
	// payload, err := github.ValidatePayload(albEvent.Headers["x-github-event"], []byte("secret"))
	// if err != nil {
	// 	log.Printf("error validating request body: err=%s\n", err)
	// 	return events.ALBTargetGroupResponse{}, nil
	// }
	// // defer albEvent.Body.Close()
	//
	// event, err := github.ParseWebHook(albEvent.Headers["x-github-event"], payload)
	// if err != nil {
	// 	log.Printf("could not parse webhook: err=%s\n", err)
	// 	// return
	// }
	//////////////////////////////////////////////////////////////////////////////////////////////////////
	DevOps := "user1, user2"
	switch e := event.(type) {
	case *github.ReleaseEvent:
		log.Println("This is a release event.")
		ReleaseAction = *e.Action
		ReleaseTag = *e.Release.TagName
		ReleaseTarget = *e.Release.TargetCommitish
		ReleaseUser = *e.Release.Author.Login
		ReleaseRepo = *e.Repo.Name

		log.Println("Release action:", ReleaseAction, "Release tag:", ReleaseTag, "Repo:", "Release Target:", ReleaseTarget, "ReleaseRepo ", ReleaseRepo, " ReleaseUser:", ReleaseUser)
	default:
		log.Println("Nothing to do for event type ", albEvent.Headers["x-github-event"])
		return HealthCheck()
	}
	log.Println("Check against the following conditions: Release is created Release Tag is: not empty It was created by a member of the DevOps")
	log.Println("We got > Release is ", ReleaseAction, " Release Tag is: ", ReleaseTag, "It was created by ", ReleaseUser, " on ", ReleaseRepo, " repository.")

	///////////////////////////////////////////////////////////////////////////////////
	// Deploy base on the following conditions:
	// 1.  If a release tag is
	//			create on master,
	//			its created by a member of devteam,
	//			has a value,i.e not empty
	// NOTE: we wont trigger deployment if a releasetag gets deleted
	///////////////////////////////////////////////////////////////////////////////////

	if ReleaseAction == "created" && ReleaseTag != "" && ReleaseTarget == "master" && strings.Contains(DevOps, ReleaseUser) {
		log.Println(ReleaseRepo + ":v" + ReleaseTag + " deployment triggered by " + ReleaseUser)

		// Jobs - JENKINS_USER:JENKINS_USER_API@JENKINS_URL/job/JOB_NAME/buildWithParameters?token=JOB_TOKEN&parameter=release_tag
		// JENKINS_USER, JENKINS_USER_API, JENKINS_URL are obtained from the secret manager
		Jobs := "http://" + jenkinsToken["USER"] + ":" + jenkinsToken["API"] + jenkinsToken["URL"] + JenkinsProdJobs(ReleaseRepo) + ReleaseTag
		log.Println(Jobs, "...")
		alertMessage = ":lambda::jenkins: deployment " + ReleaseRepo + ":v" + ReleaseTag + " triggered."
		bolatito.SendSimpleMessageToSlack(channelID, alertMessage, "jenkins", slackToken["SLACK_TOKEN"])
		
		

		DeployToJenkins(Jobs)

		log.Println("Completed.")
	}

	return HealthCheck()
}

// JenkinsProdJobs - map function containing all production jenkins job associated with their respective github repository
// Note: The jenkins' job must have been configured to be able to triggered remotely
func JenkinsProdJobs(ReleaseRepo string) string {
	AllJobs := make(map[string]string)
	AllJobs["tgithub-repo"] = "/job/git-tag/buildWithParameters?token=yesme&release_tag="
	return AllJobs[ReleaseRepo]
}

func main() {
	log.Println("Starting handleWebhook...")
	slackToken = SlackSetup()
	jenkinsToken = JenkinsSetup()
	lambda.Start(handleWebhook)
	log.Println("Finished executing handleWebhook.")
}
