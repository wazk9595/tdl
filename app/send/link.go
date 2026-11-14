package send

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Target struct {
	Peer      string
	Candidate int
	Message   int
	Explicit  bool
	Self      bool
}

func ParseTarget(link string) (Target, error) {
	if link == "me" || link == "self" || link == "saved" {
		return Target{Self: true}, nil
	}
	u, err := url.Parse(link)
	if err != nil {
		return Target{}, err
	}
	if (u.Scheme != "https" && u.Scheme != "http") || (u.Host != "t.me" && u.Host != "telegram.me") || u.User != nil {
		return Target{}, fmt.Errorf("请提供 Telegram 话题链接，或使用 me 发送到收藏夹")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) > 0 && parts[0] == "c" {
		if len(parts) < 3 || len(parts) > 4 {
			return Target{}, fmt.Errorf("私人话题链接格式无效")
		}
		if _, err := strconv.ParseUint(parts[1], 10, 63); err != nil || parts[1] == "0" {
			return Target{}, fmt.Errorf("群 ID 无效")
		}
		parts = parts[1:]
	}
	if len(parts) < 2 || len(parts) > 3 || parts[0] == "" {
		return Target{}, fmt.Errorf("链接缺少话题或消息 ID")
	}
	number := func(s string) (int, error) {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 || int64(n) > 2147483647 {
			return 0, fmt.Errorf("话题或消息 ID 无效：%s", s)
		}
		return n, nil
	}
	id, err := number(parts[1])
	if err != nil {
		return Target{}, err
	}
	t := Target{Peer: parts[0], Candidate: id, Message: id}
	if len(parts) == 3 {
		t.Message, err = number(parts[2])
		if err != nil {
			return Target{}, err
		}
		t.Explicit = true
	}
	if value := u.Query().Get("thread"); value != "" {
		thread, err := number(value)
		if err != nil {
			return Target{}, err
		}
		if t.Explicit && thread != t.Candidate {
			return Target{}, fmt.Errorf("链接中的话题信息互相矛盾")
		}
		t.Candidate = thread
		t.Explicit = true
	}
	return t, nil
}
