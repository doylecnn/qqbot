package main

import (
	"database/sql"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"
	"syscall"

	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/sirupsen/logrus"
	zero "github.com/wdvxdr1123/ZeroBot"
	"github.com/wdvxdr1123/ZeroBot/driver"
	"github.com/wdvxdr1123/ZeroBot/message"

	fuzzy "github.com/doylecnn/go-fuzzywuzzy"
	hanyuwordle "github.com/doylecnn/qqbot/hanyu_wordle"
	mylog "github.com/doylecnn/qqbot/log"
	"github.com/jmoiron/sqlx"
	"github.com/mattn/go-sqlite3"
	"github.com/pelletier/go-toml"
)

var (
	db             *sqlx.DB
	config         *toml.Tree
	groupcdseconds time.Duration

	groupLastActive map[int64]time.Time = make(map[int64]time.Time)
)

func main() {
	rand.Seed(time.Now().UnixNano())
	var configfile = "config.toml"
	if _, err := os.Stat(configfile); os.IsNotExist(err) {
		if f, err := os.Create(configfile); err != nil {
			mylog.Log.WithFields(logrus.Fields{
				"event": "Start",
				"err":   err,
			}).Warningln("创建基础文件失败")
		} else {
			f.Close()
		}
	}
	var err error
	config, err = toml.LoadFile(configfile)
	if err != nil {
		mylog.Log.WithFields(logrus.Fields{
			"event":      "Start",
			"err":        err,
			"configfile": configfile,
		}).Warningln("加载配置文件失败")
	}
	groupcdseconds, err = time.ParseDuration(config.Get("robirt.groupcdtime").(string))
	if err != nil {
		mylog.Log.WithFields(logrus.Fields{
			"event": "Start",
			"err":   err,
		}).Warningln("ParseDuration error")
	}
	sqlite3Config := config.Get("sqlite3").(*toml.Tree)
	sql.Register("sqlite3_custom", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			return conn.RegisterFunc("partial_ratio", fuzzy.PartialRatio, true)
		},
	})
	connStr := fmt.Sprintf("file:%s", sqlite3Config.Get("file").(string))
	originDB, err := sql.Open("sqlite3_custom", connStr)
	if err != nil {
		mylog.Log.WithFields(logrus.Fields{
			"event": "Start",
			"err":   err,
		}).Warningln("数据库链接失败")
	}
	db = sqlx.NewDb(originDB, "sqlite3")
	mylog.Log.WithFields(logrus.Fields{
		"event": "Start",
	}).Infoln()

	zero.Run(zero.Config{
		NickName:      []string{"bot"},
		CommandPrefix: "/",
		SuperUsers:    []int64{00000000},
		Driver: []zero.Driver{
			driver.NewWebSocketClient("ws://127.0.0.1:6700/", ""),
		},
	})
	zero.OnCommand("test").Handle(func(ctx *zero.Ctx) {
		ctx.Send(message.Text("success"))
	})
	zero.OnCommand("handle", zero.OnlyGroup).Handle(hanyuwordle.GameStart)
	zero.OnCommand("stop", zero.OnlyGroup).Handle(hanyuwordle.GameStop)
	zero.OnCommand("restart", zero.AdminPermission).Handle(hanyuwordle.BotRestart)

	zero.OnCommand("frp start", zero.SuperUserPermission).Handle(func(ctx *zero.Ctx) {
		if _, err = os.Stat("c:\\frp\\frpc.exe"); err != nil && os.IsNotExist(err) {
			source, err := os.Open("c:\\frp\\frpc.bak")
			if err != nil {
				logrus.Println(err)
				ctx.Send(message.Text(fmt.Sprintf("error:%s", err)))
			}
			defer source.Close()

			destination, err := os.Create("c:\\frp\\frpc.exe")
			if err != nil {
				logrus.Println(err)
				ctx.Send(message.Text(fmt.Sprintf("error:%s", err)))
			}
			defer destination.Close()
			_, err = io.Copy(destination, source)
			if err != nil {
				logrus.Println(err)
				ctx.Send(message.Text(fmt.Sprintf("error:%s", err)))
			}
		}
		err := exec.Command("c:\\frp\\frpc.exe", "-c", "c:\\frp\\frpc.ini").Start()
		if err != nil {
			ctx.Send(message.Text(fmt.Sprintf("error:%s", err)))
		} else {
			ctx.Send(message.Text("success"))
		}
	})

	zero.OnCommand("frp stop", zero.SuperUserPermission).Handle(func(ctx *zero.Ctx) {
		err := exec.Command("pskill", "frpc").Start()
		if err != nil {
			ctx.Send(message.Text(fmt.Sprintf("error:%s", err)))
		} else {
			ctx.Send(message.Text("success"))
		}
	})

	zero.OnRegex(`^([摸贴打撞抱舔亲扑揍扇踢推])(pupu|噗噗)$`, zero.OnlyGroup).Handle(func(ctx *zero.Ctx) {
		if v, ok := ctx.State["regex_matched"]; ok {
			a := v.([]string)
			var dongzuo string = a[1]
			var chenghu string = a[2]
			var d11 int = int(rand.Int31n(10)) + 1
			var d12 int = int(rand.Int31n(100)) + 1
			if d12 < 6 {
				d12 = 6
			}
			var s1 = roll(d11, d12)
			reply_msg := fmt.Sprintf("你 roll 了一次 [%dd%d] = %d\n", d11, d12, s1)
			var d21 int = int(rand.Int31n(10)) + 1
			var d22 int = int(rand.Int31n(100)) + 1
			if d22 < 6 {
				d12 = 6
			}
			var s2 = roll(d21, d22)
			reply_msg += fmt.Sprintf("%s roll 了一次 [%dd%d] = %d\n", chenghu, d21, d22, s2)
			if s1 < s2 {
				reply_msg += fmt.Sprintf("你的袭击被%s躲过了", chenghu)
			} else if s1 == s2 {
				reply_msg += fmt.Sprintf("你的企图被%s发现了，但为时已晚，%s还是被你%s到了", chenghu, chenghu, dongzuo)
			} else {
				reply_msg += fmt.Sprintf("你成功的%s到了%s", dongzuo, chenghu)
			}
			ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.Text(reply_msg)))
		}
	})

	zero.OnCommand("roll", zero.OnlyGroup).Handle(func(ctx *zero.Ctx) {
		s := roll(1, 6)
		ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.Text(fmt.Sprintf("[1d6] = %d", s))))
		ctx.Block()
	})

	zero.OnRegex(`^([sS沙鲨傻][bBdD逼雕]|伞兵)(\p{Han}+|\x{1F427}+)`).Handle(func(ctx *zero.Ctx) {
		if v, ok := ctx.State["regex_matched"]; ok {
			a := v.([]string)
			ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.Text(fmt.Sprintf("%s确实大%s", a[2], a[1]))))
			ctx.Block()
		}
	})

	// zero.OnRequest(func(ctx *zero.Ctx) bool {
	// 	return ctx.Event.RequestType == "friend" ||
	// 		(ctx.Event.RequestType == "group" &&
	// 			(ctx.Event.SubType == "invite" || ctx.Event.SubType == "add"))
	// }).FirstPriority().Handle(func(ctx *zero.Ctx) {
	// 	if ctx.Event.RequestType == "friend" {
	// 		ctx.SetFriendAddRequest(ctx.Event.Flag, true, "")
	// 	} else if ctx.Event.SubType == "add" {
	// 		ctx.SetGroupAddRequest(ctx.Event.Flag, "add", true, "")
	// 	} else {
	// 		ctx.SetGroupAddRequest(ctx.Event.Flag, "invite", true, "")
	// 	}
	// })

	zero.OnRegex(`^([lL垃辣][jJgG圾鸡])(\p{Han}+|\x{1F427}+)`).Handle(func(ctx *zero.Ctx) {
		if v, ok := ctx.State["regex_matched"]; ok {
			a := v.([]string)
			ctx.Send(message.ReplyWithMessage(ctx.Event.MessageID, message.Text(fmt.Sprintf("%s确实太%s了", a[2], a[1]))))
			ctx.Block()
		}
	})

	zero.OnRegex(`^(\d)[dD](100|\d{1,2})$`).Handle(func(ctx *zero.Ctx) {
		if v, ok := ctx.State["regex_matched"]; ok {
			a := v.([]string)
			var d1 int = 1
			var d2 int = 6
			if td1, err := strconv.ParseInt(a[1], 10, 32); err == nil {
				d1 = int(td1)
			} else if err != nil {
				mylog.Log.Warn(err)
				return
			} else if d1 < 1 {
				return
			}
			if td2, err := strconv.ParseInt(a[2], 10, 32); err == nil {
				d2 = int(td2)
			} else if err != nil {
				mylog.Log.Warn(err)
				return
			} else if d2 < 6 {
				return
			}
			s := roll(d1, d2)
			ctx.SendGroupMessage(ctx.Event.GroupID, message.ReplyWithMessage(ctx.Event.MessageID, message.Text(fmt.Sprintf("[%dd%d] = %d", d1, d2, s))))
			ctx.Block()
		}
	})

	zero.OnCommand("无聊图").Handle(func(ctx *zero.Ctx) {
		if thread_url, pic_urls, err := jandanpic(); err == nil {
			var msgs []message.MessageSegment
			for i := 0; i < len(pic_urls); i++ {
				msgs = append(msgs, message.Image(pic_urls[i]))
			}
			msgs = append(msgs, message.Text(thread_url))
			ctx.SendChain(msgs...)
		} else {
			mylog.Log.WithFields(logrus.Fields{
				"event": "Boring Pic",
				"call":  "jandanpic",
				"err":   err,
			}).Warningln("无聊图 error")
		}
	})

	zero.OnRegex(`^\p{Han}+$`, zero.OnlyGroup).Handle(hanyuwordle.OnGuess)

	zero.OnMessage(zero.OnlyGroup).Handle(func(ctx *zero.Ctx) {
		var msg = ctx.MessageString()
		if msg == "太难了" || msg == "放弃" {
			hanyuwordle.GameStop(ctx)
		}

		if time.Until(groupLastActive[ctx.Event.GroupID].Add(groupcdseconds)).Seconds() <= 0 {
			return
		}
		groupLastActive[ctx.Event.GroupID] = time.Now()

		if msg == "&#91;视频&#93;你的QQ暂不支持查看视频短片, 请升级到最新版本后查看。" ||
			msg == "&#91;闪照&#93;请使用新版手机QQ查看闪照。" ||
			strings.Contains(msg, "[CQ::rich,text=") {
			return
		}

		if strings.Contains(msg, "[CQ:image,file=") ||
			strings.Contains(msg, "[CQ:at,qq=") {
			return
		}

		if len([]rune(msg)) < 2 {
			return
		}

		ns, err := db.PrepareNamed(`SELECT distinct reply FROM replies where group_number=:groupnum and length(keyword)>1 and partial_ratio(keyword,:msg)>50`)
		if err != nil {
			mylog.Log.WithFields(logrus.Fields{
				"event": "onGroupMsg",
				"call":  "PrepareNamedContext",
				"err":   err,
			}).Debugln("PrepareNamedContext error")
			return
		}
		replies := []string{}
		err = ns.Select(&replies, map[string]interface{}{"msg": msg, "groupnum": ctx.Event.GroupID})
		if err != nil {
			mylog.Log.WithFields(logrus.Fields{
				"event": "onGroupMsg",
				"call":  "Select",
				"err":   err,
			}).Warningln("when select get error")
		}
		if len(replies) > 0 {
			p := rand.Int31n(6)
			if p == 4 || (zero.SuperUserPermission(ctx) && p > 3) {
				if thread_url, pic_urls, err := jandanpic(); err == nil {
					var msgs []message.MessageSegment
					for i := 0; i < len(pic_urls); i++ {
						msgs = append(msgs, message.Image(pic_urls[i]))
					}
					msgs = append(msgs, message.Text(thread_url))
					ctx.SendChain(msgs...)
					return
				}
			}
			replyMessage := replies[rand.Intn(len(replies))]
			ctx.Send(message.Text(replyMessage))
		}
	})
	zero.OnMetaEvent()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	db.Close()
}

func roll(d1, d2 int) (s int32) {
	for i := 0; i < d1; i++ {
		s += rand.Int31n(int32(d2)) + 1
	}
	return
}
