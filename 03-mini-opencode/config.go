 package main
 
 import (
 	"os"
 )
 
 type Config struct {
 	APIKey   string // DeepSeek API key
 	Model    string // 模型名
 	MaxTurns int    // Agent loop 最大轮数
 	MaxHist  int    // 对话历史最大消息数（超过截断）
 }
 
 func LoadConfig() Config {
 	return Config{
 		APIKey:   os.Getenv("DEEPSEEK_API_KEY"),
 		Model:    "deepseek-chat",
 		MaxTurns: 10,
 		MaxHist:  40,
 	}
 }
