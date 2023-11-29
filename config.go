package main

import "time"

type Config struct {
	Listen  string      `yaml:"listen"`
	Login   LoginConfig `yaml:"login"`
	EndDate time.Time   `yaml:"endDate"`
}

type LoginConfig struct {
	Id          string      `yaml:"id"`
	Token       string      `yaml:"token"`
	RedirectUrl string      `yaml:"redirectUrl"`
	BaseUrl     string      `yaml:"baseUrl"`
	Guild       GuildConfig `yaml:"guild"`
}

type GuildConfig struct {
	Id    string   `yaml:"id"`
	Roles []string `yaml:"roles"`
}
