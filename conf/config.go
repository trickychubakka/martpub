package conf

type Config struct {
	Database struct {
		Host     string `mapstructure:"host"`
		User     string `mapstructure:"user"`
		Password string `mapstructure:"password"`
		Dbname   string `mapstructure:"dbname"`
		Sslmode  string `mapstructure:"sslmode"`
	}
}
