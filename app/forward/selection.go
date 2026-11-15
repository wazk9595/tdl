package forward

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const maxMessageID = 1<<31 - 1

type idInterval struct{ First, Last int }

// IDSelection stores intervals rather than expanding potentially huge ranges.
type IDSelection struct {
	Include, Exclude []idInterval
}

func parseIntervals(input string) ([]idInterval, error) {
	var intervals []idInterval
	for _, token := range strings.Split(input, ",") {
		token = strings.TrimSpace(token)
		parts := strings.Split(token, "-")
		if len(parts) < 1 || len(parts) > 2 {
			return nil, fmt.Errorf("消息 ID 格式无效：%q；请使用 100、100-160 或 100,160", token)
		}
		parse := func(value string) (int, error) {
			value = strings.TrimSpace(value)
			for _, r := range value {
				if r < '0' || r > '9' {
					return 0, fmt.Errorf("消息 ID 必须是正整数：%q", value)
				}
			}
			id, err := strconv.ParseInt(value, 10, 32)
			if err != nil || id <= 0 {
				return 0, fmt.Errorf("消息 ID 必须在 1 到 %d 之间：%q", maxMessageID, value)
			}
			return int(id), nil
		}
		first, err := parse(parts[0])
		if err != nil {
			return nil, err
		}
		last := first
		if len(parts) == 2 {
			last, err = parse(parts[1])
			if err != nil {
				return nil, err
			}
		}
		if first > last {
			return nil, fmt.Errorf("ID 范围起点不能大于终点：%q；逆序发送请使用 --desc", token)
		}
		intervals = append(intervals, idInterval{first, last})
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].First < intervals[j].First })
	var merged []idInterval
	for _, interval := range intervals {
		if len(merged) > 0 && int64(interval.First) <= int64(merged[len(merged)-1].Last)+1 {
			merged[len(merged)-1].Last = max(merged[len(merged)-1].Last, interval.Last)
		} else {
			merged = append(merged, interval)
		}
	}
	return merged, nil
}

func containsID(intervals []idInterval, id int) bool {
	i := sort.Search(len(intervals), func(i int) bool { return intervals[i].Last >= id })
	return i < len(intervals) && intervals[i].First <= id
}

func (s *IDSelection) Excluded(id int) bool { return containsID(s.Exclude, id) }
func (s *IDSelection) Includes(id int) bool {
	return containsID(s.Include, id) && !s.Excluded(id)
}

func ParseSelection(ids, exclude string) (*IDSelection, error) {
	if ids == "" {
		if exclude != "" {
			return nil, fmt.Errorf("--exclude 需要与 --ids 一起使用")
		}
		return nil, nil
	}
	include, err := parseIntervals(ids)
	if err != nil {
		return nil, err
	}
	s := &IDSelection{Include: include}
	if exclude != "" {
		s.Exclude, err = parseIntervals(exclude)
		if err != nil {
			return nil, fmt.Errorf("排除范围无效：%w", err)
		}
	}
	return s, nil
}

func ValidateSelection(opts Options) (*IDSelection, error) {
	selection, err := ParseSelection(opts.IDs, opts.Exclude)
	if err != nil {
		return nil, err
	}
	if selection != nil {
		if opts.To == "" {
			return nil, fmt.Errorf("使用 --ids 时请指定 --to 目标话题链接、群 ID 或 me")
		}
		if len(opts.From) != 1 || strings.TrimSpace(opts.From[0]) == "" || strings.Contains(opts.From[0], "://") {
			return nil, fmt.Errorf("使用 --ids 时请指定一个来源：--from me、群 ID 或用户名")
		}
	} else {
		for _, from := range opts.From {
			if isSaved(strings.TrimSpace(from)) {
				return nil, fmt.Errorf("从收藏夹批量转发时，请用 --ids 指定消息，例如 --ids 100-160")
			}
		}
	}
	return selection, nil
}
