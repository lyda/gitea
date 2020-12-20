// Copyright 2014 The Gogs Authors. All rights reserved.
// Copyright 2019 The Gitea Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"strings"

	"code.gitea.io/gitea/models"
	"code.gitea.io/gitea/modules/auth"
	"code.gitea.io/gitea/modules/base"
	"code.gitea.io/gitea/modules/build"
	"code.gitea.io/gitea/modules/context"
	"code.gitea.io/gitea/modules/log"
	"code.gitea.io/gitea/modules/setting"
	userSetting "code.gitea.io/gitea/routers/user/setting"
)

const (
	// tplSettingsOptions template path for render settings
	tplSettingsOptions base.TplName = "org/settings/options"
	// tplSettingsDelete template path for render delete repository
	tplSettingsDelete base.TplName = "org/settings/delete"
	// tplSettingsHooks template path for render hook settings
	tplSettingsHooks base.TplName = "org/settings/hooks"
	// tplSettingsLabels template path for render labels settings
	tplSettingsLabels base.TplName = "org/settings/labels"
	// tplSettingsRunners template path for render runners settings
	tplSettingsRunners base.TplName = "org/settings/runners"
)

// Settings render the main settings page
func Settings(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsOptions"] = true
	ctx.Data["CurrentVisibility"] = ctx.Org.Organization.Visibility
	ctx.Data["RepoAdminChangeTeamAccess"] = ctx.Org.Organization.RepoAdminChangeTeamAccess
	ctx.HTML(200, tplSettingsOptions)
}

// SettingsPost response for settings change submited
func SettingsPost(ctx *context.Context, form auth.UpdateOrgSettingForm) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsOptions"] = true
	ctx.Data["CurrentVisibility"] = ctx.Org.Organization.Visibility

	if ctx.HasError() {
		ctx.HTML(200, tplSettingsOptions)
		return
	}

	org := ctx.Org.Organization

	// Check if organization name has been changed.
	if org.LowerName != strings.ToLower(form.Name) {
		isExist, err := models.IsUserExist(org.ID, form.Name)
		if err != nil {
			ctx.ServerError("IsUserExist", err)
			return
		} else if isExist {
			ctx.Data["OrgName"] = true
			ctx.RenderWithErr(ctx.Tr("form.username_been_taken"), tplSettingsOptions, &form)
			return
		} else if err = models.ChangeUserName(org, form.Name); err != nil {
			if err == models.ErrUserNameIllegal {
				ctx.Data["OrgName"] = true
				ctx.RenderWithErr(ctx.Tr("form.illegal_username"), tplSettingsOptions, &form)
			} else {
				ctx.ServerError("ChangeUserName", err)
			}
			return
		}
		// reset ctx.org.OrgLink with new name
		ctx.Org.OrgLink = setting.AppSubURL + "/org/" + form.Name
		log.Trace("Organization name changed: %s -> %s", org.Name, form.Name)
	}
	// In case it's just a case change.
	org.Name = form.Name
	org.LowerName = strings.ToLower(form.Name)

	if ctx.User.IsAdmin {
		org.MaxRepoCreation = form.MaxRepoCreation
	}

	org.FullName = form.FullName
	org.Description = form.Description
	org.Website = form.Website
	org.Location = form.Location
	org.RepoAdminChangeTeamAccess = form.RepoAdminChangeTeamAccess

	visibilityChanged := form.Visibility != org.Visibility
	org.Visibility = form.Visibility

	if err := models.UpdateUser(org); err != nil {
		ctx.ServerError("UpdateUser", err)
		return
	}

	// update forks visibility
	if visibilityChanged {
		if err := org.GetRepositories(models.ListOptions{Page: 1, PageSize: org.NumRepos}); err != nil {
			ctx.ServerError("GetRepositories", err)
			return
		}
		for _, repo := range org.Repos {
			if err := models.UpdateRepository(repo, true); err != nil {
				ctx.ServerError("UpdateRepository", err)
				return
			}
		}
	}

	log.Trace("Organization setting updated: %s", org.Name)
	ctx.Flash.Success(ctx.Tr("org.settings.update_setting_success"))
	ctx.Redirect(ctx.Org.OrgLink + "/settings")
}

// SettingsAvatar response for change avatar on settings page
func SettingsAvatar(ctx *context.Context, form auth.AvatarForm) {
	form.Source = auth.AvatarLocal
	if err := userSetting.UpdateAvatarSetting(ctx, form, ctx.Org.Organization); err != nil {
		ctx.Flash.Error(err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("org.settings.update_avatar_success"))
	}

	ctx.Redirect(ctx.Org.OrgLink + "/settings")
}

// SettingsDeleteAvatar response for delete avatar on setings page
func SettingsDeleteAvatar(ctx *context.Context) {
	if err := ctx.Org.Organization.DeleteAvatar(); err != nil {
		ctx.Flash.Error(err.Error())
	}

	ctx.Redirect(ctx.Org.OrgLink + "/settings")
}

// SettingsDelete response for deleting an organization
func SettingsDelete(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsDelete"] = true

	org := ctx.Org.Organization
	if ctx.Req.Method == "POST" {
		if _, err := models.UserSignIn(ctx.User.Name, ctx.Query("password")); err != nil {
			if models.IsErrUserNotExist(err) {
				ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_password"), tplSettingsDelete, nil)
			} else {
				ctx.ServerError("UserSignIn", err)
			}
			return
		}

		if err := models.DeleteOrganization(org); err != nil {
			if models.IsErrUserOwnRepos(err) {
				ctx.Flash.Error(ctx.Tr("form.org_still_own_repo"))
				ctx.Redirect(ctx.Org.OrgLink + "/settings/delete")
			} else {
				ctx.ServerError("DeleteOrganization", err)
			}
		} else {
			log.Trace("Organization deleted: %s", org.Name)
			ctx.Redirect(setting.AppSubURL + "/")
		}
		return
	}

	ctx.HTML(200, tplSettingsDelete)
}

// Webhooks render webhook list page
func Webhooks(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsHooks"] = true
	ctx.Data["BaseLink"] = ctx.Org.OrgLink + "/settings/hooks"
	ctx.Data["Description"] = ctx.Tr("org.settings.hooks_desc")

	ws, err := models.GetWebhooksByOrgID(ctx.Org.Organization.ID, models.ListOptions{})
	if err != nil {
		ctx.ServerError("GetWebhooksByOrgId", err)
		return
	}

	ctx.Data["Webhooks"] = ws
	ctx.HTML(200, tplSettingsHooks)
}

// DeleteWebhook response for delete webhook
func DeleteWebhook(ctx *context.Context) {
	if err := models.DeleteWebhookByOrgID(ctx.Org.Organization.ID, ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteWebhookByOrgID: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.settings.webhook_deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Org.OrgLink + "/settings/hooks",
	})
}

// Labels render organization labels page
func Labels(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.labels")
	ctx.Data["PageIsOrgSettingsLabels"] = true
	ctx.Data["RequireTribute"] = true
	ctx.Data["LabelTemplates"] = models.LabelTemplates
	ctx.HTML(200, tplSettingsLabels)
}

// Runners render runner list page
func Runners(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsRunners"] = true
	ctx.Data["BaseLink"] = ctx.Org.OrgLink + "/settings"
	ctx.Data["RunnerDescription"] = ctx.Tr("org.settings.runners_desc")

	loadRunnersData(ctx)

	ctx.HTML(200, tplSettingsRunners)
}

// GiteaRunnerNewPost creates new Gitea build runner
func GiteaRunnerNewPost(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsRunners"] = true
	ctx.Data["BaseLink"] = ctx.Org.OrgLink + "/settings"
	ctx.Data["RunnerDescription"] = ctx.Tr("org.settings.runners_desc")

	if ctx.HasError() {
		loadRunnersData(ctx)

		ctx.HTML(200, tplSettingsRunners)
		return
	}

	if err := models.NewBuildRunner(ctx.Org.Organization.ID, models.GiteaRunner); err != nil {
		ctx.ServerError("NewBuildRunner", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("org.settings.add_runner_success"))
	ctx.Redirect(ctx.Org.OrgLink + "/settings/runners")
}

func loadRunnersData(ctx *context.Context) {
	var err error
	ctx.Data["Runners"], err = models.GetBuildRunnersByOwnerID(ctx.Org.Organization.ID, models.ListOptions{})
	if err != nil {
		ctx.ServerError("GetRunnersByOwnerID", err)
		return
	}
}

// DeleteRunner response for delete runner
func DeleteRunner(ctx *context.Context) {
	if br, err := models.DeleteBuildRunnerByOwnerID(ctx.Org.Organization.ID, ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteRunnerByOwnerID: " + err.Error())
	} else {
		if (br.Type == models.GiteaRunner) {
			build.InvalidateRunnerBySecret(br.Secret)
		}
		ctx.Flash.Success(ctx.Tr("org.settings.runner_deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Org.OrgLink + "/settings/runners",
	})
}
