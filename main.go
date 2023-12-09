package main

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/1f349/cache"
	"github.com/MrMelon54/exit-reload"
	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
	_ "github.com/mattn/go-sqlite3"
	"github.com/ravener/discord-oauth2"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	configFlag string
	//go:embed index.go.html
	indexGoHtml string
	//go:embed Ubuntu.woff2
	ubuntuFont []byte
	//go:embed prosperity-winter.png
	christmasLogoPng []byte
	//go:embed prosperity-winter.svg
	christmasLogoSvg []byte
	//go:embed happy-holidays.png
	happyHolidaysPng []byte
)

func loadIndexPageTemplate() (*template.Template, error) {
	return template.New("secret-santa").Parse(indexGoHtml)
}

const CustomDateFormat = "Mon, 2 Jan 2006 15:04 MST"

type PlayerData struct {
	DiscordUser string
	McUser      string
}

func main() {
	flag.StringVar(&configFlag, "conf", "config.yml", "Path to the config file")
	flag.Parse()

	wd := filepath.Dir(configFlag)

	startTime := time.Now()

	openConf, err := os.Open(configFlag)
	if err != nil {
		log.Fatalf("Failed to open '%s': %s\n", configFlag, err)
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

	stateCache := cache.New[uuid.UUID, uuid.UUID]()
	userCache := cache.New[uuid.UUID, DiscordMember]()

	pages, err := loadIndexPageTemplate()
	if err != nil {
		log.Fatalln("[Error] loadIndexPageTemplate:", err)
	}

	db, err := sql.Open("sqlite3", filepath.Join(wd, "players.db"))
	if err != nil {
		log.Fatalln("[Error] Open players.db:", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS players
(
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    mc_user      TEXT UNIQUE NOT NULL,
    discord_id   TEXT UNIQUE NOT NULL,
    discord_user TEXT UNIQUE NOT NULL
);`)
	if err != nil {
		log.Fatalln("[Error] Failed to initialise database")
	}

	secretSantaResolve := &sync.RWMutex{}
	secretSantaUsers, err := resolvePlayers(db, conf.Seed)
	if err != nil {
		log.Fatalln("Failed to resolve players:", err)
	}

	router := httprouter.New()
	router.GET("/", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		sessId := getSessionUuid(rw, req)
		rw.WriteHeader(http.StatusOK)
		user, ok := userCache.Get(sessId)
		if !ok {
			_ = pages.Execute(rw, map[string]any{
				"LoggedIn":       false,
				"ProfilePicture": "about:blank",
				"ProfileName":    "Wumpus",
				"EndDate":        conf.EndDate.Format(CustomDateFormat),
			})
			return
		}
		secretSantaResolve.RLock()
		secretPlayer, hasRegistered := secretSantaUsers[user.User.Id]
		secretSantaResolve.RUnlock()
		hasEnded := time.Now().After(conf.EndDate)
		_ = pages.Execute(rw, map[string]any{
			"LoggedIn":       true,
			"ProfilePicture": generateAvatarUrl(user, conf.Login.Guild.Id),
			"ProfileName":    user.User.Username,
			"EndDate":        conf.EndDate.Format(CustomDateFormat),
			"HasRegistered":  hasRegistered,
			"HasEnded":       hasEnded,
			"SecretPlayer":   secretPlayer.DiscordUser,
			"McPlayer":       secretPlayer.McUser,
		})
	})
	router.GET("/Ubuntu.woff2", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		http.ServeContent(rw, req, "Ubuntu.woff2", startTime, bytes.NewReader(ubuntuFont))
	})
	router.GET("/christmas-logo.png", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		http.ServeContent(rw, req, "christmas-logo.png", startTime, bytes.NewReader(christmasLogoPng))
	})
	router.GET("/christmas-logo.svg", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		http.ServeContent(rw, req, "christmas-logo.svg", startTime, bytes.NewReader(christmasLogoSvg))
	})
	router.GET("/happy-holidays.png", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		http.ServeContent(rw, req, "happy-holidays.png", startTime, bytes.NewReader(happyHolidaysPng))
	})
	router.POST("/login", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		sessId := getSessionUuid(rw, req)
		stateId := uuid.New()
		stateCache.Set(stateId, sessId, time.Now().Add(15*time.Minute))
		http.Redirect(rw, req, oauthConf.AuthCodeURL(stateId.String()), http.StatusFound)
	})
	router.POST("/logout", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		http.SetCookie(rw, &http.Cookie{
			Name:     "session-id",
			Path:     "/",
			MaxAge:   -1,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(rw, req, "/", http.StatusFound)
	})
	router.POST("/register", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		mcUser := req.FormValue("mc_user")
		if mcUser == "" {
			http.Error(rw, "Missing Minecraft username", http.StatusBadRequest)
			return
		}
		hasEnded := time.Now().After(conf.EndDate)
		if hasEnded {
			http.Error(rw, "Registration has ended", http.StatusTeapot)
			return
		}
		sessId := getSessionUuid(rw, req)
		user, ok := userCache.Get(sessId)
		if !ok {
			http.Error(rw, "Error: Not logged in", http.StatusForbidden)
			return
		}
		_, err := db.Exec(`INSERT INTO players (mc_user, discord_id, discord_user) VALUES (?, ?, ?)`, mcUser, user.User.Id, user.User.Username)
		if err != nil {
			log.Printf("Failed to register user %s - %s: %s\n", user.User.Id, user.User.Username, err)
			http.Error(rw, "Failed to register your user", http.StatusInternalServerError)
			return
		}
		secretSantaResolve.Lock()
		secretSantaUsers, _ = resolvePlayers(db, conf.Seed)
		secretSantaResolve.Unlock()
		http.Redirect(rw, req, "/", http.StatusFound)
	})
	router.GET("/callback", func(rw http.ResponseWriter, req *http.Request, params httprouter.Params) {
		sessId := getSessionUuid(rw, req)
		stateId, err := uuid.Parse(req.FormValue("state"))
		if err != nil {
			http.Error(rw, "Invalid state parameter", http.StatusBadRequest)
			return
		}
		if checkSessId, ok := stateCache.Get(stateId); !ok || sessId != checkSessId {
			http.Error(rw, "State does not match", http.StatusBadRequest)
			return
		}
		stateCache.Delete(stateId)

		token, err := oauthConf.Exchange(context.Background(), req.FormValue("code"))
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
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

		var dm DiscordMember
		j := json.NewDecoder(res.Body)
		err = j.Decode(&dm)
		if err != nil {
			http.Error(rw, "Failed to decode Discord API response", http.StatusInternalServerError)
			return
		}

		if hasRequiredRole(dm, roleMap) {
			userCache.Set(sessId, dm, time.Now().Add(12*time.Hour))
			http.Redirect(rw, req, "/", http.StatusFound)
			return
		}

		http.Error(rw, "User is missing a required role in the Discord guild", http.StatusConflict)
	})
	server := &http.Server{
		Handler: router,
		Addr:    conf.Listen,
	}
	go func() {
		log.Printf("[SecretSanta] Listening for HTTP requests on '%s'\n", server.Addr)
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Println("[SecretSanta] Listen and serve error:", err)
		}
	}()

	exit_reload.ExitReload("SecretSanta", func() {}, func() {
		_ = server.Close()
	})
}

func resolvePlayers(db *sql.DB, seed int64) (map[string]PlayerData, error) {
	a := make(map[string]PlayerData)
	query, err := db.Query("SELECT discord_id, discord_user, mc_user FROM players ORDER BY id")
	if err != nil {
		return nil, err
	}
	discordIds := make([]string, 0)
	playerData := make([]PlayerData, 0)
	for query.Next() {
		var id, discordName, mcName string
		err := query.Scan(&id, &discordName, &mcName)
		if err != nil {
			return nil, err
		}
		discordIds = append(discordIds, id)
		playerData = append(playerData, PlayerData{
			DiscordUser: discordName,
			McUser:      mcName,
		})
	}

	// prevent shuffle crashes
	if len(playerData) < 3 {
		for i := range discordIds {
			a[discordIds[i]] = PlayerData{}
		}
		return a, nil
	}

	shuffledNames := ShufflePlayerNames(playerData, seed)
	for i := range discordIds {
		a[discordIds[i]] = shuffledNames[i]
	}
	return a, query.Err()
}

func ShufflePlayerNames(a []PlayerData, seed int64) []PlayerData {
	l := len(a)
	n := ShuffledIntSlice(l, seed)
	b := make([]PlayerData, l)
	for i := range b {
		b[i] = a[n[i]]
	}
	return b
}

func hasRequiredRole(dm DiscordMember, roleMap map[string]struct{}) bool {
	for _, i := range dm.Roles {
		if _, ok := roleMap[i]; ok {
			return true
		}
	}
	return false
}

func getSessionUuid(rw http.ResponseWriter, req *http.Request) uuid.UUID {
	cookie, err := req.Cookie("session-id")
	if err == nil {
		if parse, err := uuid.Parse(cookie.Value); err == nil {
			return parse
		}
	}
	u := uuid.New()
	http.SetCookie(rw, &http.Cookie{
		Name:     "session-id",
		Value:    u.String(),
		Path:     "/",
		Expires:  time.Now().AddDate(0, 3, 0),
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	return u
}

func generateAvatarUrl(member DiscordMember, guildId string) string {
	if member.Avatar != "" {
		ext := "png"
		if strings.HasPrefix(member.Avatar, "a_") {
			ext = "gif"
		}
		return fmt.Sprintf("https://cdn.discordapp.com/guilds/%s/users/%s/avatars/%s.%s?size=512", guildId, member.User.Id, member.Avatar, ext)
	}
	if member.User.Avatar != "" {
		ext := "png"
		if strings.HasPrefix(member.User.Avatar, "a_") {
			ext = "gif"
		}
		return fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.%s?size=512", member.User.Id, member.User.Avatar, ext)
	}
	// returns 0 on error, that's all we care about
	userId, _ := strconv.ParseInt(member.User.Id, 10, 64)
	return fmt.Sprintf("https://cdn.discordapp.com/embed/avatars/%d.png?size=512", (userId>>22)%6)
}
