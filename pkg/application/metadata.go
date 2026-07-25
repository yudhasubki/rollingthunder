package application

const (
	Name                  = "Rolling Thunder"
	Identifier            = "rollingthunder"
	DatabaseClientName    = Name
	SettingsDirectoryName = "RollingThunder"
	CredentialServiceName = "RollingThunder.DatabaseProfiles"
	GitHubRepository      = "yudhasubki/rollingthunder"
)

func UserAgent(version string) string {
	return SettingsDirectoryName + "/" + version
}
