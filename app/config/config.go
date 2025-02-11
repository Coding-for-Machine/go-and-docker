package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
	_ "github.com/joho/godotenv/autoload"
	_ "github.com/spf13/viper"
)

type Config struct {
	Postgres Postgres
}

type Postgres struct {
	Host     string
	Port     string
	User     string
	Passwd   string
	Database string
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}
	godotenv.Load("somerandomfile")
	godotenv.Load("filenumberone.env", "filenumbertwo.env")
	s3Bucket := os.Getenv("S3_BUCKET")
	secretKey := os.Getenv("SECRET_KEY")

	// now do something with s3 or whatever
}
