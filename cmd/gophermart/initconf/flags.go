package initconf

import (
	"flag"
	"fmt"
	"github.com/spf13/viper"
	"log"
	"log/slog"
	"martnew/conf"
	"net"
	"os"
	"strconv"
	"strings"
)

type LoggingConf struct {
	Logger   *slog.Logger
	LocalRun bool
	LogLevel slog.Level
}

type Config struct {
	RunAddr        string
	Logfile        string
	DatabaseDSN    string
	UseDBConfig    bool
	Key            string
	LogConf        LoggingConf
	AccrualRunAddr string
}

// IsValidIP функция для проверки на то, что строка является валидным ip адресом
func IsValidIP(ip string) bool {
	res := net.ParseIP(ip)
	return res != nil
}

// FlagTest флаг режима тестирования для отключения парсинга командной строки при тестировании
var FlagTest = false

func readDBConfig() (string, error) {
	dbCfg := &conf.Config{}
	var connStr string
	log.Println("flags and DATABASE_DSN env are not defined, trying to find and read dbconfig.yaml")
	viper.SetConfigName("dbconfig")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./conf")
	viper.AutomaticEnv()
	err := viper.ReadInConfig()
	if err != nil {
		log.Println("Error reading conf file :", err)
		return "", err
	} else {
		err = viper.Unmarshal(&dbCfg)
		if err != nil {
			log.Println("Error unmarshalling conf :", err)
			return "", err
		}
	}

	connStr = fmt.Sprintf("postgres://%s:%s@%s:5432/%s?sslmode=%s", dbCfg.Database.User, dbCfg.Database.Password, dbCfg.Database.Host, dbCfg.Database.Dbname, dbCfg.Database.Sslmode)
	return connStr, nil
}

func InitConfig(conf *Config) error {

	var logLevel string
	log.Println("start parsing flags")

	if !FlagTest {
		log.Println("start parsing flags (after if !FlagTest)")
		flag.StringVar(&conf.RunAddr, "a", "localhost:8080", "address and port to run server. Default localhost:8080.")
		flag.StringVar(&conf.Logfile, "f", "", "server log file. Default empty.")
		// Ключ для определения локального запуска приложения
		flag.BoolVar(&conf.LogConf.LocalRun, "t", true, "local run flag, development/test or production mode. Default on prod - false.")
		//flag.BoolVar(&conf.LogConf.LocalRun, "r", false, "local run flag. Default false.")
		flag.StringVar(&logLevel, "l", "Debug", "log level. Default in production -- Info. Possible values: Debug, Info, Warn, Error")
		//flag.StringVar(&conf.DatabaseDSN, "d", "", "database DSN in format postgres://user:password@host:port/dbname?sslmode=disable. Default is empty.")
		flag.StringVar(&conf.DatabaseDSN, "d", "postgres://testuser:123456@192.168.1.100:5432/testdb?sslmode=disable", "database DSN in format postgres://user:password@host:port/dbname?sslmode=disable. Default is empty.")
		//flag.StringVar(&conf.DatabaseDSN, "d", "postgresql://postgres:postgres@postgres/praktikum?sslmode=disable", "database DSN in format postgres://user:password@host:port/dbname?sslmode=disable. Default is empty.")
		flag.StringVar(&conf.Key, "k", "", "Key. Default empty.")
		flag.BoolVar(&conf.UseDBConfig, "c", false, "true/false flag -- use dbconfig/config yaml file (conf/dbconfig.yaml). Default false.")
		//flag.StringVar(&conf.AccrualRunAddr, "r", "http://localhost:9090", "host of accrual system. Default localhost.")
		flag.StringVar(&conf.AccrualRunAddr, "r", "", "host of accrual system. Default localhost.")
		flag.Parse()
	}

	log.Println("Config before env var processing:", conf)

	// Пытаемся прочитать переменную окружения ADDRESS. Переменные окружения имеют приоритет перед флагами,
	// поэтому переопределяет опции командной строки в случае, если соответствующая переменная определена в env
	log.Println("Trying to read ADDRESS environment variable (env has priority over flags): ", os.Getenv("ADDRESS"))
	if envRunAddr := os.Getenv("ADDRESS"); envRunAddr != "" {
		fmt.Println("Using env var ADDRESS:", envRunAddr)
		conf.RunAddr = envRunAddr
	}

	// Проверка на то, что заданный адрес является валидным сочетанием IP:порт
	ipPort := strings.Split(conf.RunAddr, ":")
	// адрес состоит из сочетания хост:порт
	if len(ipPort) != 2 || ipPort[1] == "" {
		return fmt.Errorf("invalid ADDRESS variable `%s`", conf.RunAddr)
	}
	// Порт содержит только цифры
	if _, err := strconv.Atoi(ipPort[1]); err != nil {
		return fmt.Errorf("invalid ADDRESS variable `%s`", conf.RunAddr)
	}
	// Если часть URI является валидным IP
	if IsValidIP(ipPort[0]) {
		log.Println("conf.runAddr is IP address, Using IP:", conf.RunAddr)
	}

	log.Println("Trying to read ACCRUAL_SYSTEM_ADDRESS environment variable (env has priority over flags): ", os.Getenv("ACCRUAL_SYSTEM_ADDRESS"))
	if envAccrualRunAddr := os.Getenv("ACCRUAL_SYSTEM_ADDRESS"); envAccrualRunAddr != "" {
		fmt.Println("Using env var ADDRESS:", envAccrualRunAddr)
		conf.AccrualRunAddr = envAccrualRunAddr
	}

	if envLogFileFlag := os.Getenv("SERVER_LOG"); envLogFileFlag != "" {
		log.Println("env var SERVER_LOG was specified, use SERVER_LOG =", envLogFileFlag)
		conf.Logfile = envLogFileFlag
		log.Println("Using env var SERVER_LOG=", envLogFileFlag)
	}

	// Ключ или переменная окружения для определения локального запуска приложения
	// Переменная LOCAL_RUN может иметь значения 1, t, true, TRUE -- 0, f, false, FALSE и т.п.
	if envLocalRun := os.Getenv("LOCAL_RUN"); envLocalRun != "" {
		log.Println("env var LOCAL_RUN was specified, use LOCAL_RUN =", envLocalRun)
		var err error
		conf.LogConf.LocalRun, err = strconv.ParseBool(envLocalRun)
		if err != nil {
			log.Println("invalid LOCAL_RUN variable ", envLocalRun)
			conf.LogConf.LocalRun = false
		}
		log.Println("Using env var LOCAL_RUN=", envLocalRun)
	}
	log.Println("LocalRun config is :", conf.LogConf.LocalRun)

	if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel != "" {
		logLevel = envLogLevel
	}
	log.Println("env var LOG_LEVEL was specified, use LOG_LEVEL =", logLevel)
	switch logLevel {
	case "Debug":
		conf.LogConf.LogLevel = slog.LevelDebug
	case "Info":
		conf.LogConf.LogLevel = slog.LevelInfo
	case "Warn":
		conf.LogConf.LogLevel = slog.LevelWarn
	case "Error":
		conf.LogConf.LogLevel = slog.LevelError
	// В случае ошибки в формате переменной окружения -- предупреждение и выставление уровня по умолчанию
	default:
		log.Println("invalid LOG_LEVEL variable format", logLevel, ". Set default logLevel Info")
		conf.LogConf.LogLevel = slog.LevelInfo
	}

	if envDatabaseDSN := os.Getenv("DATABASE_DSN"); envDatabaseDSN != "" {
		log.Println("env var DATABASE_DSN was specified, use DATABASE_DSN =", envDatabaseDSN)
		conf.DatabaseDSN = envDatabaseDSN
		log.Println("Using env var DATABASE_DSN=", conf.DatabaseDSN)
	}

	// Если DatabaseDSN нет в переменных окружения и в параметрах запуска -- пытаемся прочитать из dbconfig.yaml
	if conf.DatabaseDSN == "" && conf.UseDBConfig {
		log.Println("flags and DATABASE_DSN env are not defined, trying to find and read dbconfig.yaml")
		if connStr, err := readDBConfig(); err != nil {
			log.Println("Error reading dbconfig.yaml:", err)
		} else {
			conf.DatabaseDSN = connStr
		}
	}

	if envKey := os.Getenv("KEY"); envKey != "" {
		log.Println("env var KEY was specified, use KEY =", envKey)
		conf.Key = envKey
		log.Println("Using key")
	}

	// Logger initialization
	if _, err := SetLogger(conf); err != nil {
		log.Println("InitConfig, error in SetLogger:", err)
		return err
	}
	logger := conf.LogConf.Logger
	logger.Info("Current logging layout:", "LogLevel", conf.LogConf.LogLevel, "LocalRun", conf.LogConf.LocalRun)
	logger.Debug("DEBUG message")
	logger.Info("INFO message")
	logger.Warn("WARN message")
	logger.Error("ERROR message")

	log.Println("InitConfig: Config is", conf)
	return nil
}
