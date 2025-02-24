// Copyright [2023] [Argus]
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package v1

import (
	"encoding/json"
	"net/url"

	command "github.com/release-argus/Argus/commands"
	"github.com/release-argus/Argus/config"
	"github.com/release-argus/Argus/notifiers/shoutrrr"
	"github.com/release-argus/Argus/service"
	deployedver "github.com/release-argus/Argus/service/deployed_version"
	latestver "github.com/release-argus/Argus/service/latest_version"
	"github.com/release-argus/Argus/service/latest_version/filter"
	"github.com/release-argus/Argus/util"
	api_type "github.com/release-argus/Argus/web/api/types"
	"github.com/release-argus/Argus/webhook"
)

// getParam from a URL query string
func getParam(queryParams *url.Values, param string) *string {
	if queryParams.Has(param) {
		val := queryParams.Get(param)
		return &val
	}
	return nil
}

// announceDelete of a Service to the `s.AnnounceChannel`
// (Broadcast to all WebSocket clients).
func (api *API) announceDelete(serviceID string) {
	payloadData, _ := json.Marshal(api_type.WebSocketMessage{
		Page:    "APPROVALS",
		Type:    "DELETE",
		SubType: serviceID,
		Order:   &api.Config.Order})
	*api.Config.HardDefaults.Service.Status.AnnounceChannel <- payloadData
}

// announceEdit of a Service to the `s.AnnounceChannel` if data shown to the user has changed.
// (Broadcast to all WebSocket clients).
func (api *API) announceEdit(oldData *api_type.ServiceSummary, newData *api_type.ServiceSummary) {
	serviceChanged := ""
	if oldData != nil {
		serviceChanged = oldData.ID
		newData.RemoveUnchanged(oldData)
	}

	payloadData, _ := json.Marshal(api_type.WebSocketMessage{
		Page:        "APPROVALS",
		Type:        "EDIT",
		SubType:     serviceChanged,
		ServiceData: newData})

	// Announce all edits so that caches are refreshed
	*api.Config.HardDefaults.Service.Status.AnnounceChannel <- payloadData
}

//
// Defaults
//

// convertAndCensorDefaults will convert Defaults to API Type and censor secrets.
func convertAndCensorDefaults(input *config.Defaults) (defaults *api_type.Defaults) {
	if input == nil {
		return
	}

	// Convert to API Type, censoring secrets
	defaults = &api_type.Defaults{
		Service: api_type.ServiceDefaults{
			Options: &api_type.ServiceOptions{
				Interval:           input.Service.Options.Interval,
				SemanticVersioning: input.Service.Options.SemanticVersioning},
			LatestVersion: &api_type.LatestVersionDefaults{
				AccessToken:       util.DefaultOrValue(input.Service.LatestVersion.AccessToken, "<secret>"),
				AllowInvalidCerts: input.Service.LatestVersion.AllowInvalidCerts,
				UsePreRelease:     input.Service.LatestVersion.UsePreRelease,
				Require:           convertAndCensorLatestVersionRequireDefaults(&input.Service.LatestVersion.Require)},
			DeployedVersionLookup: &api_type.DeployedVersionLookup{
				AllowInvalidCerts: input.Service.DeployedVersionLookup.AllowInvalidCerts},
			Dashboard: &api_type.DashboardOptions{
				AutoApprove: input.Service.Dashboard.AutoApprove}},
		Notify:  *convertAndCensorNotifySliceDefaults(&input.Notify),
		WebHook: *convertAndCensorWebHookDefaults(&input.WebHook)}
	return
}

//
// Latest Version
//

// convertAndCensorLatestVersionRequireDefaults will convert RequireDefaults to API Type and censor secrets.
func convertAndCensorLatestVersionRequireDefaults(require *filter.RequireDefaults) (apiRequire *api_type.LatestVersionRequireDefaults) {
	if require == nil {
		return
	}

	apiRequire = &api_type.LatestVersionRequireDefaults{
		Docker: api_type.RequireDockerCheckDefaults{
			Type: require.Docker.Type}}

	// Docker
	//   GHCR
	if require.Docker.RegistryGHCR != nil {
		apiRequire.Docker.GHCR = &api_type.RequireDockerCheckRegistryDefaults{
			Token: util.ValueIfNotDefault(
				require.Docker.RegistryGHCR.Token, "<secret>")}
	}
	//   Hub
	if require.Docker.RegistryHub != nil {
		apiRequire.Docker.Hub = &api_type.RequireDockerCheckRegistryDefaultsWithUsername{
			Username: require.Docker.RegistryHub.Username,
			RequireDockerCheckRegistryDefaults: api_type.RequireDockerCheckRegistryDefaults{
				Token: util.ValueIfNotDefault(
					require.Docker.RegistryHub.Token, "<secret>")}}
	}
	//   Quay
	if require.Docker.RegistryQuay != nil {
		apiRequire.Docker.Quay = &api_type.RequireDockerCheckRegistryDefaults{
			Token: util.ValueIfNotDefault(
				require.Docker.RegistryQuay.Token, "<secret>")}
	}
	return
}

// convertAndCensorLatestVersionRequire will convert Require to API Type and censor secrets.
func convertAndCensorLatestVersionRequire(require *filter.Require) (apiRequire *api_type.LatestVersionRequire) {
	if require == nil {
		return
	}

	var docker *api_type.RequireDockerCheck
	if require.Docker != nil {
		docker = &api_type.RequireDockerCheck{
			Type:     require.Docker.Type,
			Image:    require.Docker.Image,
			Tag:      require.Docker.Tag,
			Username: require.Docker.Username,
			Token:    util.ValueIfNotDefault(require.Docker.Token, "<secret>")}
	}

	apiRequire = &api_type.LatestVersionRequire{
		Command:      require.Command,
		Docker:       docker,
		RegexContent: require.RegexContent,
		RegexVersion: require.RegexVersion}
	return
}

// convertURLCommandSlice will convert URLCommandSlice to API Type.
func convertURLCommandSlice(commands *filter.URLCommandSlice) *api_type.URLCommandSlice {
	if commands == nil {
		return nil
	}
	slice := make(api_type.URLCommandSlice, len(*commands))
	for index := range *commands {
		slice[index] = api_type.URLCommand{
			Type:     (*commands)[index].Type,
			Regex:    (*commands)[index].Regex,
			Index:    (*commands)[index].Index,
			Template: (*commands)[index].Template,
			Text:     (*commands)[index].Text,
			Old:      (*commands)[index].Old,
			New:      (*commands)[index].New}
	}
	return &slice
}

// convertAndCensorLatestVersion will convert Lookup to API Type and censor secrets.
func convertAndCensorLatestVersion(lv *latestver.Lookup) (apiLV *api_type.LatestVersion) {
	if lv == nil {
		return
	}

	apiLV = &api_type.LatestVersion{
		Type:              lv.Type,
		URL:               lv.URL,
		AccessToken:       util.DefaultOrValue(lv.AccessToken, "<secret>"),
		AllowInvalidCerts: lv.AllowInvalidCerts,
		UsePreRelease:     lv.UsePreRelease,
		URLCommands:       convertURLCommandSlice(&lv.URLCommands),
		Require:           convertAndCensorLatestVersionRequire(lv.Require)}

	return
}

//
// Deployed Version
//

// convertAndCensorDeployedVersionLookup will convert Lookup to API Type and censor secrets.
func convertAndCensorDeployedVersionLookup(dvl *deployedver.Lookup) (apiDVL *api_type.DeployedVersionLookup) {
	if dvl == nil {
		return
	}
	var headers []api_type.Header
	apiDVL = &api_type.DeployedVersionLookup{
		URL:               dvl.URL,
		AllowInvalidCerts: dvl.AllowInvalidCerts,
		Headers:           headers,
		JSON:              dvl.JSON,
		Regex:             dvl.Regex,
		RegexTemplate:     dvl.RegexTemplate}
	// Basic auth
	if dvl.BasicAuth != nil {
		apiDVL.BasicAuth = &api_type.BasicAuth{
			Username: dvl.BasicAuth.Username,
			Password: "<secret>"}
	}
	// Headers
	apiDVL.Headers = make([]api_type.Header, len(dvl.Headers))
	for i := range dvl.Headers {
		apiDVL.Headers[i] = api_type.Header{
			Key:   dvl.Headers[i].Key,
			Value: "<secret>"}
	}
	return
}

//
// Notify
//

// convertAndCensorNotifySliceDefaults will convert Slice to NotifySlice and censor secrets.
func convertAndCensorNotifySliceDefaults(input *shoutrrr.SliceDefaults) *api_type.NotifySlice {
	if input == nil {
		return nil
	}

	// Convert to API Type, censoring secrets
	slice := make(api_type.NotifySlice, len(*input))
	for name := range *input {
		slice[name] = (&api_type.Notify{
			Type:      (*input)[name].Type,
			Options:   (*input)[name].Options,
			URLFields: (*input)[name].URLFields,
			Params:    (*input)[name].Params})
		slice[name].Censor()
	}
	return &slice
}

// convertAndCensorNotifySlice will convert Slice to API Type and censor secrets.
func convertAndCensorNotifySlice(input *shoutrrr.Slice) *api_type.NotifySlice {
	if input == nil {
		return nil
	}

	// Convert to API Type, censoring secrets
	slice := make(api_type.NotifySlice, len(*input))
	for name := range *input {
		slice[name] = (&api_type.Notify{
			Type:      (*input)[name].Type,
			Options:   (*input)[name].Options,
			URLFields: (*input)[name].URLFields,
			Params:    (*input)[name].Params})
		slice[name].Censor()
	}
	return &slice
}

//
// Command
//

// convertCommandSlice will convert Slice to API type.
func convertCommandSlice(commands *command.Slice) *api_type.CommandSlice {
	if commands == nil {
		return nil
	}
	slice := make(api_type.CommandSlice, len(*commands))
	for index := range *commands {
		slice[index] = api_type.Command((*commands)[index])
	}
	return &slice
}

//
// WebHook
//

// convertWebHookHeaders will convert WebHook Headers to API type.
func convertWebHookHeaders(headers *webhook.Headers) (apiHeaders *[]api_type.Header) {
	if headers == nil {
		return
	}

	converted := make([]api_type.Header, len(*headers))
	for i, header := range *headers {
		converted[i] = api_type.Header{
			Key:   header.Key,
			Value: header.Value}
	}

	apiHeaders = &converted
	return
}

// convertAndCensorWebHookDefaults will convert WebHookDefaults to API type and censor the secret.
func convertAndCensorWebHookDefaults(webhook *webhook.WebHookDefaults) (apiElement *api_type.WebHook) {
	if webhook == nil {
		return
	}

	apiElement = (&api_type.WebHook{
		Type:              util.StringToPointer(webhook.Type),
		URL:               util.StringToPointer(webhook.URL),
		AllowInvalidCerts: webhook.AllowInvalidCerts,
		Secret: util.ValueIfNotNil(
			util.StringToPointer(webhook.Secret), "<secret>"),
		CustomHeaders:     convertWebHookHeaders(webhook.CustomHeaders),
		DesiredStatusCode: webhook.DesiredStatusCode,
		Delay:             webhook.Delay,
		MaxTries:          webhook.MaxTries,
		SilentFails:       webhook.SilentFails})
	apiElement.Censor()
	return
}

// convertAndCensorWebHookSliceDefaults will convert SliceDefaults to API Type and censor secrets.
func convertAndCensorWebHookSliceDefaults(input *webhook.SliceDefaults) *api_type.WebHookSlice {
	if input == nil {
		return nil
	}

	// Convert to API Type, censoring secrets
	slice := make(api_type.WebHookSlice, len(*input))
	for name := range *input {
		slice[name] = convertAndCensorWebHookDefaults((*input)[name])
	}
	return &slice
}

// convertAndCensorWebHook will convert WebHook to API type and censor the secret.
func convertAndCensorWebHook(webhook *webhook.WebHook) (apiElement *api_type.WebHook) {
	if webhook == nil {
		return
	}

	customHeaders := convertWebHookHeaders(webhook.CustomHeaders)

	apiElement = (&api_type.WebHook{
		Type:              util.StringToPointer(webhook.Type),
		URL:               util.StringToPointer(webhook.URL),
		AllowInvalidCerts: webhook.AllowInvalidCerts,
		Secret: util.ValueIfNotNil(
			util.StringToPointer(webhook.Secret), "<secret>"),
		CustomHeaders:     customHeaders,
		DesiredStatusCode: webhook.DesiredStatusCode,
		Delay:             webhook.Delay,
		MaxTries:          webhook.MaxTries,
		SilentFails:       webhook.SilentFails})
	apiElement.Censor()
	return
}

// convertAndCensorWebHookSlice will convert Slice to API Type and censor secrets.
func convertAndCensorWebHookSlice(webhooks *webhook.Slice) *api_type.WebHookSlice {
	if webhooks == nil {
		return nil
	}
	slice := make(api_type.WebHookSlice, len(*webhooks))
	for index := range *webhooks {
		slice[index] = convertAndCensorWebHook((*webhooks)[index])
	}
	return &slice
}

//
// Service
//

// convertAndCensorService will convert Service to API type and censor the secrets.
func convertAndCensorService(service *service.Service) (apiService *api_type.Service) {
	if service == nil {
		return
	}
	apiService = &api_type.Service{}

	apiService.Comment = service.Comment

	apiService.Options = &api_type.ServiceOptions{
		Active:             service.Options.Active,
		Interval:           service.Options.Interval,
		SemanticVersioning: service.Options.SemanticVersioning}

	// LatestVersion
	apiService.LatestVersion = convertAndCensorLatestVersion(&service.LatestVersion)
	// DeployedVersionLookup
	apiService.DeployedVersionLookup = convertAndCensorDeployedVersionLookup(service.DeployedVersionLookup)
	// Notify
	apiService.Notify = convertAndCensorNotifySlice(&service.Notify)
	// Command
	apiService.Command = convertCommandSlice(&service.Command)
	// WebHook
	apiService.WebHook = convertAndCensorWebHookSlice(&service.WebHook)

	apiService.Dashboard = &api_type.DashboardOptions{
		AutoApprove: service.Dashboard.AutoApprove,
		Icon:        service.Dashboard.Icon,
		IconLinkTo:  service.Dashboard.IconLinkTo,
		WebURL:      service.Dashboard.WebURL}
	return
}
