package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type DBNode struct {
	ID  int16  // origin_node value written into rows created through this node
	DSN string // postgres://user:pass@host:5432/chat
}

type Config struct {
	Port            string
	DBNodes         []DBNode
	PreferredNode   int16 // 0 = pick a random start node (spreads API tasks across DB nodes)
	DBMaxConns      int32
	AttemptTimeout  time.Duration // max time for one DB attempt before failing over
	HealthInterval  time.Duration // how often each DB node is pinged
	ShutdownTimeout time.Duration
	LogLevel        string
	RedisAddr       string // empty = do not publish realtime events (see Additional.md)
}

// Load reads the environment. DB_NODES format: "1=postgres://...,2=postgres://...,3=postgres://..."
func Load() (Config, error) {
	c := Config{
		Port:            env("PORT", "8080"),
		LogLevel:        env("LOG_LEVEL", "info"),
		RedisAddr:       os.Getenv("REDIS_ADDR"),
		DBMaxConns:      int32(envInt("DB_MAX_CONNS", 10)),
		AttemptTimeout:  envDuration("DB_ATTEMPT_TIMEOUT", 2*time.Second),
		HealthInterval:  envDuration("DB_HEALTH_INTERVAL", 2*time.Second),
		ShutdownTimeout: envDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		PreferredNode:   int16(envInt("DB_PREFERRED_NODE", 0)),
	}

	raw := os.Getenv("DB_NODES")
	if raw == "" {
		return c, fmt.Errorf("DB_NODES is required, e.g. 1=postgres://chat:chat@localhost:5432/chat")
	}
	seen := map[int16]bool{}
	for _, part := range strings.Split(raw, ",") {
		idStr, dsn, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || dsn == "" {
			return c, fmt.Errorf("DB_NODES entry %q must look like <id>=<dsn>", part)
		}
		id, err := strconv.ParseInt(idStr, 10, 16)
		if err != nil || id <= 0 {
			return c, fmt.Errorf("DB_NODES entry %q: node id must be a positive small integer", part)
		}
		if seen[int16(id)] {
			return c, fmt.Errorf("DB_NODES: duplicate node id %d", id)
		}
		seen[int16(id)] = true
		c.DBNodes = append(c.DBNodes, DBNode{ID: int16(id), DSN: dsn})
	}
	if c.PreferredNode != 0 && !seen[c.PreferredNode] {
		return c, fmt.Errorf("DB_PREFERRED_NODE=%d is not in DB_NODES", c.PreferredNode)
	}
	return c, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil {
		return v
	}
	return def
}
