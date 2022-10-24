package test

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

type Rd struct {
	// 18 length integer
	num int
	// 18 length string
	str string
}

// Grab from https//github.com/hashicorp/terraform-provider-azurerm
func NewRd() Rd {
	rand.Seed(time.Now().UnixNano())
	timeStr := strings.Replace(time.Now().Local().Format("060102150405.00"), ".", "", 1) // no way to not have a .?
	postfix := randStringFromCharSet(4, "0123456789")
	i, err := strconv.Atoi(timeStr + postfix)
	// i is of foloowing format:
	// 000000000000000000
	// YYMMddHHmmsshhRRRR
	if err != nil {
		panic(err)
	}

	str := randStringFromCharSet(18, "abcdefghijklmnopqrstuvwxyz")

	return Rd{
		num: i,
		str: str,
	}
}

func randStringFromCharSet(strlen int, charSet string) string {
	result := make([]byte, strlen)
	for i := 0; i < strlen; i++ {
		result[i] = charSet[randIntRange(0, len(charSet))]
	}
	return string(result)
}

func randIntRange(min int, max int) int {
	// #nosec G404 -- This is fine for testing
	return rand.Intn(max-min) + min
}

// RandomIntOfLength is a random 8 to 18 digit integer which is unique to this test case
func (rd Rd) RandomIntOfLength(len int) int {
	// len should not be
	//  - greater then 18, longest a int can represent
	//  - less then 8, as that gives us YYMMDDRR
	if 8 > len || len > 18 {
		panic("Invalid Test: RandomIntOfLength: len is not between 8 or 18 inclusive")
	}

	// 18 - just return the int
	if len >= 18 {
		return int(rd.num)
	}

	// 16-17 just strip off the last 1-2 digits
	if len >= 16 {
		return int(rd.num) / int(math.Pow10(18-len))
	}

	// 8-15 keep len - 2 digits and add 2 characters of randomness on
	s := strconv.Itoa(int(rd.num))
	r := s[16:18]
	v := s[0 : len-2]
	i, _ := strconv.Atoi(v + r)

	return i
}

func (rd Rd) RandomStringOfLength(len int) string {
	if len > 18 {
		panic("Invalid Test: RandomStringOfLength: len is larger than 18")
	}
	return rd.str[0:len]
}

func (rd Rd) RandomRgName() string {
	return fmt.Sprintf("aztfy-rg-%s", rd.RandomStringOfLength(8))
}
