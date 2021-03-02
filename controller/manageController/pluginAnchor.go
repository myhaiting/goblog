package manageController

import (
	"github.com/kataras/iris/v12"
	"irisweb/config"
	"irisweb/model"
	"irisweb/provider"
	"irisweb/request"
)

func PluginAnchorList(ctx iris.Context) {
	//需要支持分页，还要支持搜索
	currentPage := ctx.URLParamIntDefault("page", 1)
	pageSize := ctx.URLParamIntDefault("limit", 20)
	keyword := ctx.URLParam("keyword")

	linkList, total, err := provider.GetAnchorList(keyword, currentPage, pageSize)
	if err != nil {
		ctx.JSON(iris.Map{
			"code": config.StatusFailed,
			"msg":  "",
		})
		return
	}

	ctx.JSON(iris.Map{
		"code": config.StatusOK,
		"msg":  "",
		"count": total,
		"data": linkList,
	})
}

func PluginAnchorDetail(ctx iris.Context) {
	id := uint(ctx.URLParamIntDefault("id", 0))

	anchor, err := provider.GetAnchorById(id)
	if err != nil {
		ctx.JSON(iris.Map{
			"code": config.StatusFailed,
			"msg":  err.Error(),
		})
		return
	}

	ctx.JSON(iris.Map{
		"code": config.StatusOK,
		"msg":  "",
		"data": anchor,
	})
}

func PluginAnchorDetailForm(ctx iris.Context) {
	var req request.PluginAnchor
	if err := ctx.ReadJSON(&req); err != nil {
		ctx.JSON(iris.Map{
			"code": config.StatusFailed,
			"msg":  err.Error(),
		})
		return
	}

	var anchor *model.Anchor
	var err error

	var changeTitle bool
	var changeLink bool

	if req.Id > 0 {
		anchor, err = provider.GetAnchorById(req.Id)
		if err != nil {
			ctx.JSON(iris.Map{
				"code": config.StatusFailed,
				"msg":  err.Error(),
			})
			return
		}
		//只有旧的才需要处理
		if anchor.Title != req.Title {
			changeTitle = true
		}
		if anchor.Link != req.Link {
			changeLink = true
		}

	} else {
		anchor = &model.Anchor{
			Status: 1,
		}
	}

	anchor.Title = req.Title
	anchor.Link = req.Link
	anchor.Weight = req.Weight

	err = anchor.Save(config.DB)
	if err != nil {
		ctx.JSON(iris.Map{
			"code": config.StatusFailed,
			"msg":  err.Error(),
		})
		return
	}

	if changeTitle || changeLink {
		//锚文本名称更改了，不管连接有没有更改，都删掉旧的
		go provider.ChangeAnchor(anchor, changeTitle)
	}

	ctx.JSON(iris.Map{
		"code": config.StatusOK,
		"msg":  "链接已更新",
	})
}

func PluginAnchorReplace(ctx iris.Context) {
	var req request.PluginAnchor
	if err := ctx.ReadJSON(&req); err != nil {
		ctx.JSON(iris.Map{
			"code": config.StatusFailed,
			"msg":  err.Error(),
		})
		return
	}

	if req.Id > 0 {
		//更新单个
		anchor, err := provider.GetAnchorById(req.Id)
		if err != nil {
			ctx.JSON(iris.Map{
				"code": config.StatusFailed,
				"msg":  err.Error(),
			})
			return
		}

		go provider.ReplaceAnchor(anchor)
	} else {
		go provider.ReplaceAnchor(nil)
	}

	ctx.JSON(iris.Map{
		"code": config.StatusOK,
		"msg":  "锚文本替换任务已执行",
	})
}

func PluginAnchorDelete(ctx iris.Context) {
	var req request.PluginAnchorDelete
	if err := ctx.ReadJSON(&req); err != nil {
		ctx.JSON(iris.Map{
			"code": config.StatusFailed,
			"msg":  err.Error(),
		})
		return
	}

	if req.Id > 0 {
		//删一条
		anchor, err := provider.GetAnchorById(req.Id)
		if err != nil {
			ctx.JSON(iris.Map{
				"code": config.StatusFailed,
				"msg":  err.Error(),
			})
			return
		}

		err = provider.DeleteAnchor(anchor)
		if err != nil {
			ctx.JSON(iris.Map{
				"code": config.StatusFailed,
				"msg":  err.Error(),
			})
			return
		}
	} else if len(req.Ids) > 0 {
		//删除多条
		for _, id := range req.Ids {
			anchor, err := provider.GetAnchorById(id)
			if err != nil {
				continue
			}

			_ = provider.DeleteAnchor(anchor)
		}
	}

		ctx.JSON(iris.Map{
		"code": config.StatusOK,
		"msg":  "已执行删除操作",
	})
}

func PluginAnchorExport(ctx iris.Context) {
	anchors, err := provider.GetAllAnchors()
	if err != nil {
		ctx.JSON(iris.Map{
			"code": config.StatusFailed,
			"msg":  err.Error(),
		})
		return
	}

	//header
	header := []string{"title", "link", "weight"}
	var content [][]interface{}
	//content
	for _, v := range anchors {
		content = append(content, []interface{}{v.Title, v.Link, v.Weight})
	}

	ctx.JSON(iris.Map{
		"code": config.StatusOK,
		"msg":  "",
		"data": iris.Map{
			"header": header,
			"content": content,
		},
	})
}

func PluginAnchorImport(ctx iris.Context) {
	file, info, err := ctx.FormFile("file")
	if err != nil {
		ctx.JSON(iris.Map{
			"status": config.StatusFailed,
			"msg":    err.Error(),
		})
		return
	}
	defer file.Close()

	result, err := provider.ImportAnchors(file, info)
	if err != nil {
		ctx.JSON(iris.Map{
			"status": config.StatusFailed,
			"msg":    err.Error(),
		})
		return
	}

	ctx.JSON(iris.Map{
		"code": config.StatusOK,
		"msg":  "上传完毕",
		"data": result,
	})
}

func PluginAnchorSetting(ctx iris.Context) {
	pluginAnchor := config.JsonData.PluginAnchor

	ctx.JSON(iris.Map{
		"code": config.StatusOK,
		"msg":  "",
		"data": pluginAnchor,
	})
}

func PluginAnchorSettingForm(ctx iris.Context) {
	var req request.PluginAnchorSetting
	if err := ctx.ReadJSON(&req); err != nil {
		ctx.JSON(iris.Map{
			"code": config.StatusFailed,
			"msg":  err.Error(),
		})
		return
	}

	if req.AnchorDensity < 10 {
		req.AnchorDensity = 100
	}

	config.JsonData.PluginAnchor.AnchorDensity = req.AnchorDensity
	config.JsonData.PluginAnchor.ReplaceWay = req.ReplaceWay
	config.JsonData.PluginAnchor.KeywordWay = req.KeywordWay

	err := config.WriteConfig()
	if err != nil {
		ctx.JSON(iris.Map{
			"code": config.StatusFailed,
			"msg":  err.Error(),
		})
		return
	}

	ctx.JSON(iris.Map{
		"code": config.StatusOK,
		"msg":  "配置已更新",
	})
}