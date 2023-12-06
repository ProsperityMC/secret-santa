package main

import "time"

type Config struct {
	Listen string      `yaml:"listen"`
	Login  LoginConfig `yaml:"login"`
	// EndDate uses RFC3339 format: 2006-01-02T15:04:05Z
	EndDate time.Time `yaml:"endDate"`
	Seed    int64     `yaml:"seed"`
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
