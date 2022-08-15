package main

import (
	"errors"
	"math/rand"
	"net/http"

	"github.com/PuerkitoBio/goquery"
)

func jandanpic() (string, []string, error) {
	req, err := http.NewRequest("GET", "https://jandan.net/top", nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/89.0.4389.90 Safari/537.36")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode != 200 {
		return "", nil, errors.New(resp.Status)
	}
	d, err := goquery.NewDocumentFromReader(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return "", nil, err
	}
	var boringPics [][]string
	var boringUrls []string
	d.Find("ol.commentlist li").Each(func(_ int, s *goquery.Selection) {
		var picurls []string
		s.Find("div.text p img").Each(func(_ int, img *goquery.Selection) {
			if v, exists := img.Attr("org_src"); exists {
				picurls = append(picurls, "https:"+v)
			} else if v, exists := img.Attr("src"); exists {
				picurls = append(picurls, "https:"+v)
			}
		})
		if len(picurls) > 0 {
			boringPics = append(boringPics, picurls)
			if v, exists := s.Find("ol.commentlist li div.text span.righttext a").Attr("href"); exists {
				boringUrls = append(boringUrls, "https://jandan.net"+v)
			}
		}
	})
	if len(boringPics) > 0 {
		var randIdx = rand.Intn(len(boringPics))
		return boringUrls[randIdx], boringPics[randIdx], nil
	} else {
		return "", nil, errors.New("no boring picture")
	}
}
