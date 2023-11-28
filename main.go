package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/ProsperityMC/secret-santa/utils"
	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
	"github.com/ravener/discord-oauth2"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

func main() {
	openConf, err := os.Open(".data/config.yml")
	if err != nil {
		log.Fatal("Failed to open '.data/config.yml':", err)
	}
	var conf Config
	err = yaml.NewDecoder(openConf).Decode(&conf)
	if err != nil {
		log.Fatal("Failed to decode config:", err)
	}

	roleMap := make(map[string]struct{})
	for _, i := range conf.Login.Guild.Roles {
		roleMap[i] = struct{}{}
	}

	oauthConf := &oauth2.Config{
		RedirectURL:  conf.Login.RedirectUrl,
		ClientID:     conf.Login.Id,
		ClientSecret: conf.Login.Token,
		Scopes:       []string{discord.ScopeIdentify, "guilds.members.read"},
		Endpoint:     discord.Endpoint,
	}

	allowedStates := make(map[string]struct{})
	statesLock := &sync.RWMutex{}

	router := httprouter.New()
	router.GET("/", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		rw.WriteHeader(http.StatusOK)
		rw.Write()
		http.Error(rw, "Prosperity r/place API endpoint!", http.StatusOK)
	})
	router.GET("/login", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		u := uuid.NewString()
		statesLock.Lock()
		allowedStates[u] = struct{}{}
		statesLock.Unlock()
		http.Redirect(rw, req, oauthConf.AuthCodeURL(u), http.StatusTemporaryRedirect)
	})
	router.GET("/callback", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		z := req.FormValue("state")
		statesLock.RLock()
		if _, ok := allowedStates[z]; !ok {
			statesLock.RUnlock()
			rw.WriteHeader(http.StatusBadRequest)
			_, _ = rw.Write([]byte("State does not match."))
			return
		}
		statesLock.RUnlock()
		statesLock.Lock()
		delete(allowedStates, z)
		statesLock.Unlock()

		token, err := oauthConf.Exchange(context.Background(), req.FormValue("code"))
		if err != nil {
			rw.WriteHeader(http.StatusInternalServerError)
			_, _ = rw.Write([]byte(err.Error()))
			return
		}

		res, err := oauthConf.Client(context.Background(), token).Get("https://discord.com/api/users/@me/guilds/" + conf.Login.Guild.Id + "/member")
		if err != nil {
			http.Error(rw, "Error collecting data from the Discord API", http.StatusInternalServerError)
			return
		}

		// check request status code
		switch res.StatusCode {
		case 200:
			break
		case 404:
			http.Error(rw, "User must be in the Discord guild", http.StatusConflict)
			return
		default:
			http.Error(rw, "Received unexpected response from Discord", http.StatusInternalServerError)
			return
		}

		defer func(Body io.ReadCloser) {
			_ = Body.Close()
		}(res.Body)

		var dm utils.DiscordMember
		j := json.NewDecoder(res.Body)
		err = j.Decode(&dm)
		if err != nil {
			http.Error(rw, "Failed to decode Discord API response", http.StatusInternalServerError)
			return
		}

		for _, i := range dm.Roles {
			if _, ok := roleMap[i]; ok {
				goto hasRole
			}
		}

		http.Error(rw, "User is missing a required role in the Discord guild", http.StatusConflict)
		return

	hasRole:
		// no need for the client to get the roles
		dm.Roles = nil

		dcToken, _ := encryptDiscordTokens(&privKey.PublicKey, token)

		u := uuid.NewString()
		h, err := signer.GenerateJwt(u, u, time.Hour*24, utils.DiscordInfo{UserId: dm.User.Id, Name: dm.User.Username, Discord: dcToken})
		if err != nil {
			http.Error(rw, "Failed to generate JWT token", http.StatusInternalServerError)
		}

		_, _ = fmt.Fprintf(rw, "<!DOCTYPE html><html><head><script>window.onload=function(){window.opener.postMessage(")
		encoder := json.NewEncoder(rw)
		_ = encoder.Encode(map[string]any{
			"token":  map[string]string{"access": h},
			"member": dm,
		})
		_, _ = fmt.Fprintf(rw, ",\"%s\");window.close();}</script></head></html>", conf.Login.BaseUrl)
	})
	server := &http.Server{
		Handler: router,
		Addr:    conf.Listen,
	}
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Println("[Main] Listen and serve error:", err)
		}
	}()
}
