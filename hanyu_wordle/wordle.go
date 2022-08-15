package hanyuwordle

import (
	"bufio"
	"bytes"
	"embed"
	"errors"
	"fmt"
	"image/png"
	"math"
	"math/rand"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/doylecnn/qqbot/log"
	"github.com/sirupsen/logrus"

	"github.com/fogleman/gg"
	"github.com/mozillazg/go-pinyin"
	"github.com/samber/lo"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/message"
)

//go:embed wordle_dicts
var dictsDir embed.FS

type GameStatus int

var (
	Ready GameStatus = 0
	Start GameStatus = 1
	End   GameStatus = 2
)

type Games struct {
	games  map[int64]*Game
	status string
	mux    sync.RWMutex
}

type Game struct {
	Status    GameStatus
	Answer    Answer
	GuessList []Guess
	guesses   map[string]Guess
	Count     int
	Tips      []rune
	Mux       sync.Mutex
}

type Guess struct {
	UserName string
	Word     string
	PinYin   [][4]string
	Tag      [4]string
}

type Answer struct {
	Word   Word
	PinYin [][4]string
}

type Word struct {
	Text string
	Type string
}

var dict map[int][]Word = make(map[int][]Word)
var reZhongWenWord = regexp.MustCompile(`^\p{Han}+$`)
var games Games = Games{games: make(map[int64]*Game), mux: sync.RWMutex{}, status: "ready"}
var pinyinArgs = pinyin.Args{Style: pinyin.Tone3, Heteronym: false}

func wordleDictionaryInit() {
	THUOCLDicts := []string{"动物", "财经", "汽车", "地名", "食物", "IT", "法律", "人名", "医药", "成语", "古诗文"}
	for _, dn := range THUOCLDicts {
		data, err := dictsDir.ReadFile(fmt.Sprintf("wordle_dicts/THUOCL_%s.txt", dn))
		if err != nil {
			log.Log.WithFields(logrus.Fields{
				"event":    "Handle Game Init",
				"Error":    err,
				"DictName": dn,
			}).Fatalln("字典初始出错")
		}
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			word := strings.TrimSpace(strings.Split(scanner.Text(), "\t")[0])
			if reZhongWenWord.MatchString(word) {
				length := len([]rune(word))
				dict[length] = append(dict[length], Word{word, dn})
			}
		}
	}

	otherDicts := []string{"维基百科", "萌娘百科"}
	for _, dn := range otherDicts {
		data, err := dictsDir.ReadFile(fmt.Sprintf("wordle_dicts/dict_%s.txt", dn))
		if err != nil {
			log.Log.WithFields(logrus.Fields{
				"event":    "Handle Game Init",
				"Error":    err,
				"DictName": dn,
			}).Fatalln("字典初始出错")
		}
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			word := strings.TrimSpace(strings.Split(scanner.Text(), "\t")[0])
			if reZhongWenWord.MatchString(word) {
				length := len([]rune(word))
				dict[length] = append(dict[length], Word{word, dn})
			}
		}
	}
	for k, v := range dict {
		dict[k] = lo.Shuffle(v)
	}
}

func BotRestart(ctx *zero.Ctx) {
	games.mux.Lock()
	defer games.mux.Unlock()
	if games.status == "ready" {
		games.status = "end"
		for groupid := range games.games {
			ctx.SendGroupMessage(groupid, message.Text("准备重启啦"))
		}
	}
}

func GameStart(ctx *zero.Ctx) {
	if len(dict) == 0 {
		wordleDictionaryInit()
	}
	rlocker := games.mux.RLocker()
	rlocker.Lock()
	if games.status == "end" {
		rlocker.Unlock()
		ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.Text("在准备重启啦，请稍后再开")))
		return
	}
	_, exists := games.games[ctx.Event.GroupID]
	rlocker.Unlock()
	if exists {
		ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.Text("已经开始啦")))
	} else if games.mux.TryLock() {
		log.Log.WithFields(logrus.Fields{
			"event":     "Handle Game Start",
			"Msg":       ctx.MessageString(),
			"QQGroupId": ctx.Event.GroupID,
		}).Infoln("游戏开始")
		game := &Game{Status: Ready, guesses: make(map[string]Guess), Mux: sync.Mutex{}}
		games.mux.Unlock()
		err := firstGuess(ctx, game)
		if err != nil {
			if err.Error() == "内容太长了，换一个短点的词！" {
				ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.Text(err)))
				ctx.Block()
			}
			return
		}
		if game.Status != End {
			games.mux.Lock()
			games.games[ctx.Event.GroupID] = game
			games.mux.Unlock()
		}
		if game.Status == Ready {
			ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.Text("游戏开始啦，下一条群消息就是第一次猜测（并确定词的长度）\n\n灰色: 不太对\n黄色: 位置不太对\n绿色: 对对对\n灰色拼音元素: 排除\n\n输入“太难了”、“放弃”或者/stop指令结束游戏并看答案")))
		}
	}
	ctx.Block()
}

func GameStop(ctx *zero.Ctx) {
	rlocker := games.mux.RLocker()
	rlocker.Lock()
	game, exists := games.games[ctx.Event.GroupID]
	rlocker.Unlock()
	if exists {
		games.mux.Lock()
		delete(games.games, ctx.Event.GroupID)
		games.mux.Unlock()
		urlstr := "https://www.bing.com/search?q=" + url.QueryEscape(game.Answer.Word.Text)
		if game.Answer.Word.Type == "moegirl" {
			urlstr = "https://www.bing.com/search?q=site%3Azh.moegirl.org.cn+\"" + url.PathEscape(game.Answer.Word.Text) + "\""
		}
		ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.Text(fmt.Sprintf("本轮终止\n 答案是：%[1]s\n所以… %[1]s 是什么呢？好吃吗？ Bing一下: %[2]s", game.Answer.Word.Text, urlstr))))
	}
}

func OnGuess(ctx *zero.Ctx) {
	rlocker := games.mux.RLocker()
	rlocker.Lock()
	game, exists := games.games[ctx.Event.GroupID]
	rlocker.Unlock()
	if !exists {
		return
	} else if game.Status == Ready {
		err := firstGuess(ctx, game)
		if err != nil {
			if err.Error() == "非常不巧，词典里没有这个长度的词……" {
				ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.Text(err)))
				ctx.Block()
			}
			return
		}
		ctx.Block()
	} else if game.Status != End {
		msg := strings.TrimSpace(ctx.MessageString())
		length := len([]rune(msg))
		if length != len([]rune(game.Answer.Word.Text)) {
			return
		}
		if _, exists := game.guesses[msg]; exists {
			ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.Text("这个词已经猜过啦")))
			ctx.Block()
			return
		}

		guessPinYin := makePinYin(msg)
		if len(guessPinYin) == 0 {
			return
		}
		game.Mux.Lock()
		imageBytes, err := guess(game, ctx, msg, guessPinYin, game.Answer.PinYin)
		if err != nil {
			log.Log.WithFields(logrus.Fields{
				"event":        "Handle Game Guess",
				"Error":        err,
				"Game":         game,
				"Guess":        msg,
				"GuessPinYin":  guessPinYin,
				"Answer":       game.Answer.Word.Text,
				"AnswerDict":   game.Answer.Word.Type,
				"AnswerPinYin": game.Answer.PinYin,
			}).Warningln("猜测过程异常")
		}
		if game.Answer.Word.Text == msg {
			game.Status = End
			games.mux.Lock()
			games.games[ctx.Event.GroupID] = game
			delete(games.games, ctx.Event.GroupID)
			games.mux.Unlock()
			urlstr := "https://www.bing.com/search?q=" + url.QueryEscape(game.Answer.Word.Text)
			if game.Answer.Word.Type == "moegirl" {
				urlstr = "https://www.bing.com/search?q=site%3Azh.moegirl.org.cn+\"" + url.PathEscape(game.Answer.Word.Text) + "\""
			}
			ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.ImageBytes(imageBytes), message.Text(fmt.Sprintf("（总共 %[1]d 次）猜对啦！答案是:\n%[2]s\n所以… %[2]s 是什么呢？好吃吗？ Bing 一下: %[3]s", game.Count, game.Answer.Word.Text, urlstr))))
			game.Mux.Unlock()
			ctx.Block()
			return
		} else if len(game.GuessList) > 9 && len(game.GuessList)%5 == 0 {
			var guessedWords []rune
			for _, v := range game.GuessList {
				guessedWords = append(guessedWords, []rune(v.Word)...)
			}
			guessedWords = append(guessedWords, game.Tips...)
			guessedWords = lo.Uniq(guessedWords)
			hints := []rune(game.Answer.Word.Text)
			hints = lo.DropWhile(hints, func(r rune) bool {
				return lo.Contains(guessedWords, r)
			})
			hints = lo.Uniq(hints)
			hints = lo.Shuffle(hints)
			if len(hints) > 0 {
				game.Tips = append(game.Tips, hints[0])
				game.Tips = lo.Uniq(game.Tips)
			}
		}
		if len(game.Tips) > 0 {
			ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.ImageBytes(imageBytes), message.Text(fmt.Sprintf("词条来源：%s, 第 %d 次\n提示 包含以下几个字：%s", game.Answer.Word.Type, game.Count, string(game.Tips)))))
		} else {
			ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.ImageBytes(imageBytes), message.Text(fmt.Sprintf("词条来源：%s, 第 %d 次", game.Answer.Word.Type, game.Count))))
		}
		game.Mux.Unlock()
		ctx.Block()
	}
}

func firstGuess(ctx *zero.Ctx, game *Game) (err error) {
	if !game.Mux.TryLock() {
		return
	}
	defer game.Mux.Unlock()
	msg := strings.TrimSpace(ctx.MessageString())
	if strings.HasPrefix(msg, "/handle") {
		msg = strings.TrimSpace(msg[7:])
	}
	if !reZhongWenWord.MatchString(msg) {
		return
	}
	length := len([]rune(msg))
	if length < 2 {
		length = 2
	}
	if length > 9 {
		length = 9
	}
	selectedDict, exists := dict[length]
	if !exists {
		err = errors.New("非常不巧，词典里没有这个长度的词……")
		return
	}
	guessPinYin := makePinYin(msg)
	if len(guessPinYin) == 0 {
		return
	}
	answer := selectedDict[rand.Intn(len(dict[length]))]
	answerPinYin := makePinYin(answer.Text)
	game.Answer = Answer{Word: answer, PinYin: answerPinYin}
	game.Status = Start
	imageBytes, err := guess(game, ctx, msg, guessPinYin, game.Answer.PinYin)
	if err != nil {
		log.Log.WithFields(logrus.Fields{
			"event":        "Handle Game Guess",
			"Error":        err,
			"Game":         game,
			"Guess":        msg,
			"GuessPinYin":  guessPinYin,
			"Answer":       game.Answer.Word.Text,
			"AnswerDict":   game.Answer.Word.Type,
			"AnswerPinYin": game.Answer.PinYin,
		}).Warningln("猜测过程异常")
	}
	log.Log.WithFields(logrus.Fields{
		"event":     "Handle Game First Guess",
		"Answer":    game.Answer,
		"QQGroupId": ctx.Event.GroupID,
	}).Infoln("第一次猜测")
	if game.Answer.Word.Text == msg {
		game.Status = End
		games.mux.Lock()
		games.games[ctx.Event.GroupID] = game
		delete(games.games, ctx.Event.GroupID)
		games.mux.Unlock()
		urlstr := "https://www.bing.com/search?q=" + url.QueryEscape(game.Answer.Word.Text)
		if game.Answer.Word.Type == "moegirl" {
			urlstr = "https://www.bing.com/search?q=site%3Azh.moegirl.org.cn+\"" + url.PathEscape(game.Answer.Word.Text) + "\""
		}
		ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.ImageBytes(imageBytes), message.Text(fmt.Sprintf("（总共 %[1]d 次）猜对啦！答案是:\n%[2]s\n所以… %[2]s 是什么呢？好吃吗？ Bing 一下: %[3]s", game.Count, game.Answer.Word.Text, urlstr))))
	} else {
		ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.ImageBytes(imageBytes), message.Text(fmt.Sprintf("词条分类：%s, 第 %d 次", game.Answer.Word.Type, game.Count))))
	}
	return
}

func guess(game *Game, ctx *zero.Ctx, msg string, guessPinYin, targetPinYin [][4]string) (imageBytes []byte, err error) {
	tag := pinYinMatch(game, guessPinYin, targetPinYin)
	guess := Guess{UserName: ctx.CardOrNickName(ctx.Event.UserID), Word: msg, PinYin: guessPinYin, Tag: tag}
	game.GuessList = append(game.GuessList, guess)
	game.guesses[msg] = guess
	game.Count++
	games.mux.Lock()
	games.games[ctx.Event.GroupID] = game
	games.mux.Unlock()
	// 画图
	img, err := drawGameBorad(game)
	imageBytes = img.Bytes()
	return
}

func drawGameBorad(game *Game) (boardImage *bytes.Buffer, err error) {
	size := len([]rune(game.Answer.Word.Text))
	realTotal := float64(game.Count) + math.Ceil(29/float64(size))/4
	width := (96*size+8)*int(math.Ceil(realTotal/16.0)) - 8
	height := int(math.Ceil(96 * math.Min(realTotal, 16.0)))
	best := make(map[string]int)
	log.Log.WithFields(logrus.Fields{
		"event":  "Handle Game Draw Image",
		"Width":  width,
		"Height": height,
	}).Debugln("Draw Image")
	dc := gg.NewContext(width, height)
	dc.SetRGB(1, 1, 1)
	dc.Clear()
	filePath1 := "C:\\Windows\\Fonts\\msyhbd.ttc"
	face1, err := gg.LoadFontFace(filePath1, 52)
	if err != nil {
		panic(err)
	}
	defer face1.Close()
	filePath2 := "C:\\Windows\\Fonts\\arialnb.ttf"
	face2, err := gg.LoadFontFace(filePath2, 22)
	if err != nil {
		panic(err)
	}
	defer face2.Close()
	face3, err := gg.LoadFontFace(filePath2, 18)
	if err != nil {
		panic(err)
	}
	defer face3.Close()
	backgroundColor := []string{"#f7f8f9", "#f7f8f9", "#1d9c9c"}
	frontColor := []string{"#5d6572", "#de7525", "#ffffff"}
	ColorMap := [][]string{
		{"#b4b8be", "#de7525", "#1d9c9c"},
		{"#b4b8be", "#de7525", "#1d9c9c"},
		{"#5d6572", "#de7525", "#ffffff"},
	}
	left := 0.0
	top := 0.0
	for _, guess := range game.GuessList {
		for i := 0; i < size; i++ {
			pinyin1 := guess.PinYin[i][1]
			pinyin2 := guess.PinYin[i][2]
			pinyin3 := guess.PinYin[i][3]
			tag1 := int(guess.Tag[1][i] - 48)
			tag2 := int(guess.Tag[2][i] - 48)
			tag3 := int(guess.Tag[3][i] - 48)
			if _, exists := best[pinyin1]; !exists {
				best[pinyin1] = 0
			}
			if _, exists := best[pinyin2]; !exists {
				best[pinyin2] = 0
			}
			if best[pinyin1] < tag1 {
				best[pinyin1] = tag1
			}
			if best[pinyin2] < tag2 {
				best[pinyin2] = tag2
			}

			dc.SetHexColor(backgroundColor[int(guess.Tag[0][i])-48])
			dc.DrawRectangle(left+float64(i*96)+2, top+2, 94.0, 94.0)
			dc.Fill()

			dc.SetHexColor(frontColor[int(guess.Tag[0][i])-48])
			dc.SetFontFace(face1)
			dc.DrawStringAnchored(guess.PinYin[i][0], left+float64(i*96)+48, top+60, 0.5, 0.5)

			if len(pinyin1+pinyin2+pinyin3) <= 6 {
				dc.SetFontFace(face2)
			} else {
				dc.SetFontFace(face3)
			}

			w1, _ := dc.MeasureString(strings.ToUpper(pinyin1))
			w2, _ := dc.MeasureString(strings.ToUpper(pinyin2))
			w3, _ := dc.MeasureString(guess.PinYin[i][3])
			w1 /= 2
			w2 /= 2
			w3 /= 2
			colormap := ColorMap[int(guess.Tag[0][i]-48)]

			dc.SetHexColor(colormap[tag1])
			dc.DrawStringAnchored(strings.ToUpper(pinyin1), left+float64(i*96)+48-w2-w3, top+20, 0.5, 0.5)
			dc.SetHexColor(colormap[tag2])
			dc.DrawStringAnchored(strings.ToUpper(pinyin2), left+float64(i*96)+48+w1-w3, top+20, 0.5, 0.5)
			dc.SetHexColor(colormap[tag3])
			dc.DrawStringAnchored(pinyin3, left+float64(i*96)+48+w1+w2, top+20, 0.5, 0.5)
		}
		top += 96

		if top == 1536 {
			left += float64(96*size) + 8
			top = 0
		}
	}

	part := []string{
		"b", "c", "ch", "d", "f", "g", "h", "j", "k", "l", "m", "n", "p", "q", "r", "s", "sh", "t", "w", "x", "y", "z", "zh",
		"",
		"a", "ai", "an", "ang", "ao",
		"e", "ei", "en", "eng", "er",
		"i", "ia", "ian", "iang", "iao", "ie", "in", "ing", "iong", "iu",
		"o", "ong", "ou",
		"u", "ua", "uai", "uan", "uang", "ue", "ui", "un", "uo",
		"v", "ve",
	}
	face4, err := gg.LoadFontFace(filePath2, 15)
	if err != nil {
		panic(err)
	}
	defer face4.Close()

	colormap2 := []string{"#5d6572", "#de7525", "#1d9c9c"}

	for i, v := range part {
		j := i % (size * 2)
		k := math.Floor(float64(i) / float64(size*2))

		if top+k*24 == 1536 {
			left += 96*float64(size) + 8
			top = -k * 24
		}

		vLen := len(v)
		if vLen > 0 {
			if vLen <= 3 {
				dc.SetFontFace(face3)
			} else {
				dc.SetFontFace(face4)
			}

			if v, exists := best[part[i]]; exists {
				dc.SetHexColor(colormap2[v])
				dc.DrawRectangle(left+float64(j)*48+1, top+k*24+1, 46, 22)
				dc.Fill()
				dc.SetHexColor("#ffffff")
				dc.DrawStringAnchored(strings.ToUpper(part[i]), left+float64(j)*48+24, top+k*24+12, 0.5, 0.5)
			} else {
				dc.SetHexColor("#5d6572")
				dc.DrawRectangle(left+float64(j)*48+2, top+k*24+2, 46, 22)
				dc.Fill()
				dc.SetHexColor("#ffffff")
				dc.DrawRectangle(left+float64(j)*48+1, top+k*24+1, 46, 22)
				dc.Fill()
				dc.SetHexColor("#5d6572")
				dc.DrawStringAnchored(strings.ToUpper(part[i]), left+float64(j)*48+24, top+k*24+12, 0.5, 0.5)
			}
		}
	}

	boardImage = new(bytes.Buffer)
	err = png.Encode(boardImage, dc.Image())
	return
}

func pinYinMatch(game *Game, guessPinYin, targetPinYin [][4]string) (tag [4]string) {
	for i := 0; i < 4; i++ {
		var guessCounts map[string]int = make(map[string]int)
		var answerCounts map[string]int = make(map[string]int)
		var pos []int
		for j := 0; j < len([]rune(game.Answer.Word.Text)); j++ {
			if guessPinYin[j][i] == targetPinYin[j][i] {
				pos = append(pos, 0)
			} else {
				guessCounts[guessPinYin[j][i]] = guessCounts[guessPinYin[j][i]] + 1
				answerCounts[targetPinYin[j][i]] = answerCounts[targetPinYin[j][i]] + 1
				pos = append(pos, guessCounts[guessPinYin[j][i]])
			}
		}

		for j := 0; j < len([]rune(game.Answer.Word.Text)); j++ {
			if pos[j] > 0 {
				if v, exists := answerCounts[guessPinYin[j][i]]; exists && pos[j] <= v {
					tag[i] += "1"
				} else {
					tag[i] += "0"
				}
			} else {
				tag[i] += "2"
			}
		}
	}
	return
}

var rePinYinElements = regexp.MustCompile(`^([bcdfghjklmnpqrstwxyz]|ch|sh|zh|)([aeiouv]+(?:n|ng|)|n|ng|er)(\d?)$`)

func makePinYin(word string) (result [][4]string) {
	pinYinSymbols := pinyin.Pinyin(word, pinyinArgs)
	if len(pinYinSymbols) == len([]rune(word)) {
		for i, w := range []rune(word) {
			if rePinYinElements.MatchString(pinYinSymbols[i][0]) {
				match := rePinYinElements.FindStringSubmatch(pinYinSymbols[i][0])

				if match[2] == "n" {
					match[2] = "en"
				}

				if match[2] == "ng" {
					match[2] = "eng"
				}

				result = append(result, [4]string{string(w), match[1], match[2], match[3]})
			}
		}
	}
	log.Log.WithFields(logrus.Fields{
		"event":         "Handle Game MakePinYin",
		"PinYinSymbols": result,
	}).Debugln("MakePinYin")
	return
}
