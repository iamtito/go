package bolatito

import (
	"fmt"

	"github.com/nlopes/slack"
)

//SendSimpleMessageToSlack posts a chat message to the passed in Channel.
func SendSimpleMessageToSlack(channelID string, message string, botName string, slackToken string) error {
	if len(slackToken) == 0 {
		return fmt.Errorf("no SLACK_TOKEN passed into method")
	}

	api := slack.New(slackToken)

	_, _, err := api.PostMessage(channelID, slack.MsgOptionText(message, true), slack.MsgOptionUsername(botName))
	if err != nil {
		fmt.Printf("%s\n", err)
		return err
	}
	return nil
}
