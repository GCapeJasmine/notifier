package kafka

type IAuthConfig interface {
	GetUrl() string
	GetUsername() string
	GetPassword() string
	GetAuthentication() string
	GetTimeout() int
}

type AuthConfig struct {
	Url            string `yaml:"url" mapstructure:"url"`
	Username       string `yaml:"username" mapstructure:"username" json:"-"`
	Password       string `yaml:"password" mapstructure:"password" json:"-"`
	Authentication string `yaml:"authentication" mapstructure:"authentication"`
	Timeout        int    `yaml:"timeout" mapstructure:"timeout"`
}

func (a AuthConfig) GetUrl() string {
	return a.Url
}

func (a AuthConfig) GetUsername() string {
	return a.Username
}

func (a AuthConfig) GetPassword() string {
	return a.Password
}

func (a AuthConfig) GetAuthentication() string {
	return a.Authentication
}

func (a AuthConfig) GetTimeout() int {
	return a.Timeout
}
