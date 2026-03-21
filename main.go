package main

import (
	"gdrive-bot/bot"
	"gdrive-bot/core"
	"gdrive-bot/storage"
)

func main() {

	storage.Init()

	bot.Init()

	go core.StartWorkers(3)

	bot.Start()

	select {}
}
