package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/api/googleapi/transport"
	"google.golang.org/api/youtube/v3"

	"github.com/bwmarrin/discordgo"
	"github.com/senseyeio/duration"
	"github.com/sirupsen/logrus"
)

const (
	command = "!rec"
)

var (
	// DiscordToken for discordgo
	DiscordToken *string

	// YouTube API Token
	ytToken *string

	youtubeService *youtube.Service
	stopBot        = make(chan struct{})
)

func init() {
	DiscordToken = flag.String("discord", "", "Discord APP token. e.g. NTQwXX...")
	ytToken = flag.String("youtube", "", "YouTube APP token. e.g. AIza...")
	flag.Parse()

	if *DiscordToken == "" || *ytToken == "" {
		logrus.Panicf("Insufficient args...")
	}

	*DiscordToken = "Bot " + *DiscordToken

	youtubeService = initYoutubeService()

	customFormatter := new(logrus.TextFormatter)
	customFormatter.TimestampFormat = "2006-01-02 15:04:05"
	logrus.SetFormatter(customFormatter)
	customFormatter.FullTimestamp = true
	logrus.SetLevel(logrus.InfoLevel)
}

func initYoutubeService() *youtube.Service {
	return newYoutubeService(newClient())
}

func newClient() *http.Client {
	client := &http.Client{
		Transport: &transport.APIKey{Key: *ytToken},
	}
	return client
}

func newYoutubeService(client *http.Client) *youtube.Service {
	service, err := youtube.New(client)
	if err != nil {
		logrus.Fatalf("Unable to create YouTube service: %v", err)
	}

	return service
}

func getVideoID(s string) (string, error) {
	// TODO: parse non-URL youtube ID

	// Check URL schema...
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("Failed to parse URL : %s", err.Error())
	}
	q := u.Query()
	videoID := q.Get("v")
	if videoID != "" {
		// return if ?v=... query found...
		return videoID, nil
	}
	if strings.HasPrefix(s, "https://youtu.be/") {
		// return if s begin with https://youtu.be ...
		return strings.TrimPrefix(s, "https://youtu.be/"), nil
	}
	return "", fmt.Errorf("URL Schema not valid")
}

func messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		// Ignore bots
		return
	}
	commands := strings.Split(m.Content, " ")
	if len(commands) != 2 {
		return
	}
	if commands[0] != command {
		return
	}
	videoID, err := getVideoID(commands[1])
	if err != nil {
		sendError(s, m, err)
		return
	}

	call := youtubeService.Videos.List([]string{"id", "snippet", "contentDetails"}).Id(videoID).MaxResults(1)
	resp, err := call.Do()
	if err != nil {
		sendError(s, m, err)
		return
	}
	if len(resp.Items) != 1 {
		sendError(s, m, fmt.Errorf("Item not found"))
		return
	}
	item := resp.Items[0]

	duration, _ := duration.ParseISO8601(item.ContentDetails.Duration)
	publishedAt, _ := time.Parse(time.RFC3339, item.Snippet.PublishedAt)
	rec := recommend{
		title:       item.Snippet.Title,
		URL:         fmt.Sprintf("https://www.youtube.com/watch?v=%s", item.Id),
		imageURL:    item.Snippet.Thumbnails.High.Url,
		channelName: item.Snippet.ChannelTitle,
		// channelThumbnailURL: item.Snippet.ChannelId, // TODO: solve channel thumbnail
		channelURL:  fmt.Sprintf("https://www.youtube.com/channel/%s", item.Snippet.ChannelId),
		description: item.Snippet.Description,
		duration:    duration,
		publishedAt: publishedAt,
	}
	// logrus.Debugf("rec :", rec)
	if err := sendRecommend(s, m, rec); err != nil {
		logrus.Errorf("ERROR :", err)
	}
}

type recommend struct {
	title               string
	URL                 string
	imageURL            string
	channelName         string
	channelThumbnailURL string
	channelURL          string
	description         string
	duration            duration.Duration
	publishedAt         time.Time
}

func sendError(s *discordgo.Session, m *discordgo.MessageCreate, e error) error {
	embed := &discordgo.MessageEmbed{
		Timestamp:   time.Now().Format(time.RFC3339), // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.
		Title:       "ERROR",
		Description: e.Error(),
		Color:       0xff0000, // RED?
		Fields: []*discordgo.MessageEmbedField{{
			Name:   "コマンド送信者",
			Value:  fmt.Sprintf("<@%s>", m.Author.ID),
			Inline: false,
		}},
	}
	_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
	if err != nil {
		return err
	}
	return nil
}

func sendRecommend(s *discordgo.Session, m *discordgo.MessageCreate, rec recommend) error {
	embed := &discordgo.MessageEmbed{
		URL:         rec.URL,
		Title:       rec.title,
		Type:        discordgo.EmbedTypeRich,
		Description: rec.description,                 // TODO: Limit message length
		Timestamp:   time.Now().Format(time.RFC3339), // Discord wants ISO8601; RFC3339 is an extension of ISO8601 and should be completely compatible.
		Color:       0x00ff00,                        // Green

		Image: &discordgo.MessageEmbedImage{
			URL: rec.imageURL,
		},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			// URL: rec.channelThumbnailURL,
		},
		Author: &discordgo.MessageEmbedAuthor{
			Name:    rec.channelName,
			URL:     rec.channelURL,
			IconURL: rec.channelThumbnailURL,
		},
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "チャンネル名",
				Value:  rec.channelName,
				Inline: true,
			},
			{
				Name:   "URL",
				Value:  rec.channelURL,
				Inline: true,
			},
			{
				Name:   "公開日時",
				Value:  rec.publishedAt.String(),
				Inline: false,
			},
			{
				Name:   "長さ",
				Value:  fmt.Sprintf("%d時間%d分%d秒", rec.duration.TH,rec.duration.TM, rec.duration.TS),
				Inline: true,
			},
			/*{
				Name:   "Author Thumbnail URL",
				Value:  rec.channelThumbnailURL,
				Inline: true,
			},*/
			{
				Name:   "RECOMMENDED BY",
				Value:  fmt.Sprintf("<@%s>", m.Author.ID),
				Inline: false,
			},
		},
	}
	// logrus.Debugf("emb :", embed)
	_, err := s.ChannelMessageSendEmbed(m.ChannelID, embed)
	if err != nil {
		return err
	}
	return nil
}

func main() {
	discord, err := discordgo.New()
	discord.Token = *DiscordToken
	if err != nil {
		logrus.Fatalf("Failed to initialize discord session : %v\n", err)
	}

	// メッセージを受信した時のハンドラーを追加
	discord.AddHandler(messageHandler)

	// BOT起動
	openerr := discord.Open()
	if openerr != nil {
		logrus.Fatalf("ERR : %v\n", openerr)
	}
	logrus.Infof("Logged in Discord BOT %s\n", discord.State.User.Username)
	<-stopBot //プログラムが終了しないようロック
}
