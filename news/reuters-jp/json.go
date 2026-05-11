package reutersjp

import (
	"NewsChannel/news"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

func (r *ReutersJP) getArticles(url string, topic news.Topic) ([]news.Article, error) {
	data, err := news.HttpGet(url)
	if err != nil {
		return nil, err
	}

	var root map[string]any
	err = json.Unmarshal(data, &root)
	if err != nil {
		return nil, err
	}

	// Iterate over the article block
	var articles []news.Article
	stories := root["result"].(map[string]any)["articles"].([]any)
	for _, story := range stories {
		article, err := r.createArticle(story.(map[string]any), topic)
		if err != nil {
			return nil, err
		}
		if article == nil {
			continue
		}

		articles = append(articles, *article)
		return articles, nil
	}

	return articles, nil
}

func (r *ReutersJP) createArticle(story map[string]any, topic news.Topic) (*news.Article, error) {
	title := news.SanitizeText(story["title"].(string))
	// Compare previous articles to see if we have a duplicate.
	if news.IsDuplicateArticle(r.oldArticleTitles, title) {
		return nil, nil
	}
	r.oldArticleTitles = append(r.oldArticleTitles, title)

	articlePath := story["canonical_url"]
	articleURL := fmt.Sprintf("https://jp.reuters.com/pf/api/v3/content/fetch/article-by-id-or-url-v1?query={\"website_url\":\"%s\",\"website\":\"reuters-japan\"}", articlePath)
	articleData, err := news.HttpGet(articleURL)
	if err != nil {
		return nil, err
	}

	// Parse article JSON
	var articleJSON map[string]any
	err = json.Unmarshal(articleData, &articleJSON)
	if err != nil {
		var serr *json.SyntaxError
		if errors.As(err, &serr) {
			return nil, nil
		}

		return nil, err
	}

	content, locationString, err := parseArticle(articleJSON)
	if err != nil {
		return nil, err
	}

	// Possible there is no text?
	if len(*content) == 0 {
		return nil, nil
	}

	var location *news.Location
	if locationString != nil {
		splitter := func(r rune) bool {
			return r == '/' || r == '／'
		}
		location = news.GetLocationForExtractedLocation(strings.FieldsFunc(*locationString, splitter), "jp")
	} else {
		location = nil
	}

	// Finally get the thumbnail.
	thumbnail, err := getThumbnail(story)
	if err != nil {
		return nil, err
	}

	return &news.Article{
		Title:     title,
		Content:   content,
		Topic:     topic,
		Location:  location,
		Thumbnail: thumbnail,
	}, nil
}

func parseArticle(root map[string]any) (*string, *string, error) {
	var ret string

	if root["result"] == nil {
		return nil, nil, nil
	}

	article := root["result"].(map[string]any)
	if article["content_elements"] == nil {
		return nil, nil, nil
	}

	for _, content := range article["content_elements"].([]any) {
		if content.(map[string]any)["type"].(string) != "paragraph" {
			continue
		}

		// Sanitize paragraph
		unSanitized := content.(map[string]any)["content"].(string)
		sanitized := news.SanitizeText(unSanitized)

		ret += sanitized
		ret += "\n\n"
	}

	ret = strings.TrimSpace(ret)

	// Get the location
	if article["dateline"] != nil {
		datelines := article["dateline"].([]any)
		if len(datelines) > 0 {
			dateline := regexp.MustCompile(`([\[|［])(.*?)[０-９]`)
			location := dateline.FindStringSubmatch(datelines[0].(string))
			if len(location) > 2 && len(location[2]) > 0 {
				return &ret, &location[2], nil
			}
		}
	}

	return &ret, nil, nil
}

func getThumbnail(story map[string]any) (*news.Thumbnail, error) {
	// Don't add Reuters logo as image
	if story["thumbnail"].(map[string]any)["id"] != nil {
		if story["thumbnail"].(map[string]any)["id"].(string) == "466BJJQ7PVGY5O53NZ3KL65MHM" {
			return nil, nil
		}
	}

	thumbnailURL := story["thumbnail"].(map[string]any)["url"].(string)

	data, err := news.HttpGet(thumbnailURL)
	if err != nil {
		return nil, err
	}

	if len(data) == 0 {
		return nil, nil
	}

	caption := ""
	if story["thumbnail"].(map[string]any)["caption"] != nil {
		caption = story["thumbnail"].(map[string]any)["caption"].(string)
	}

	return &news.Thumbnail{
		Image:   news.ConvertImage(data),
		Caption: news.SanitizeText(caption),
	}, nil
}
