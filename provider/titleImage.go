package provider

import (
	"encoding/base64"
	"fmt"
	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/draw"
	"kandaoni.com/anqicms/config"
	"kandaoni.com/anqicms/library"
	"kandaoni.com/anqicms/model"
	"math"
	"math/rand"
	"mime/multipart"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/flopp/go-findfont"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
)

type TitleImage struct {
	PublicPath string
	config     *config.PluginTitleImageConfig
	useWebp    int
	article    *model.Archive
	font       *truetype.Font
	fontSize   int
}

func (w *Website) NewTitleImage(article *model.Archive) *TitleImage {
	rand.Seed(time.Now().UnixNano())
	f := loadLocalFont(w.PublicPath + w.PluginTitleImage.FontPath)
	fontSize := w.PluginTitleImage.FontSize
	if fontSize < 16 {
		fontSize = 32
	}
	t := TitleImage{
		PublicPath: w.PublicPath,
		config:     &w.PluginTitleImage,
		useWebp:    w.Content.UseWebp,
		article:    article,
		font:       f,
		fontSize:   fontSize,
	}

	return &t
}

func (t *TitleImage) DrawTitles(w *Website) error {
	// 先draw title
	img := t.makeBackground(t.article.Title)
	img = t.drawTitle(img, t.article.Title)
	// 开始保存
	location, err := t.Save(w, img, t.article.Title)
	if err != nil {
		return err
	}
	// 写入 article
	t.article.Images = append(t.article.Images, strings.TrimPrefix(location, w.PluginStorage.StorageUrl))
	if t.article.Id > 0 {
		w.DB.Model(t.article).UpdateColumn("images", t.article.Images)
	}
	if t.config.DrawSub && t.article.ArchiveData != nil {
		// 尝试解析h2标签
		re, _ := regexp.Compile(`(?i)<h2.*?>(.*?)</h2>`)
		result := re.FindAllStringSubmatch(t.article.ArchiveData.Content, -1)
		if len(result) == 0 {
			// 不存在h2,则尝试查找h3
			re, _ = regexp.Compile(`(?i)<h3.*?>(.*?)</h3>`)
			result = re.FindAllStringSubmatch(t.article.ArchiveData.Content, -1)
		}
		if len(result) > 0 {
			for _, v := range result {
				title := strings.ReplaceAll(library.StripTags(v[1]), "\n", " ")
				img = t.makeBackground(title)
				img = t.drawTitle(img, title)
				// 开始保存
				location, err = t.Save(w, img, title)
				if err != nil {
					continue
				}
				newString := v[0] + "\n" + "<p><img src=\"" + location + "\" alt=\"" + title + "\" /></p>"
				t.article.ArchiveData.Content = strings.Replace(t.article.ArchiveData.Content, v[0], newString, 1)
			}
		}
		// 生成完毕，写入数据库
		if t.article.Id > 0 {
			w.DB.Model(model.ArchiveData{}).Where("`id` = ?", t.article.Id).UpdateColumn("content", t.article.ArchiveData.Content)
		}
	}

	return nil
}

func (t *TitleImage) DrawPreview() string {
	if len(t.article.Title) == 0 {
		return ""
	}
	img := t.makeBackground(t.article.Title)
	img = t.drawTitle(img, t.article.Title)

	data := t.EncodeB64string(img)

	return data
}

func sumTitle(title string) string {
	str := library.Md5(title)
	var newStr = make([]byte, 0, 32)
	for i := range str {
		if str[i] > 57 {
			newStr = append(newStr, str[i]-49)
		} else {
			newStr = append(newStr, str[i])
		}
	}
	return string(newStr)
}

func (t *TitleImage) makeBackground(title string) (newImg image.Image) {
	titleSum := sumTitle(title)
	if len(t.config.BgImage) > 0 {
		file, err := os.Open(t.PublicPath + t.config.BgImage)
		defer file.Close()
		if err == nil {
			img, _, err := image.Decode(file)
			if err == nil {
				newImg = library.ThumbnailCrop(t.config.Width, t.config.Height, img, 2)
				return
			} else {
				file.Seek(0, 0)
				if strings.HasSuffix(t.config.BgImage, "webp") {
					img, err = webp.Decode(file)
					if err == nil {
						newImg = library.ThumbnailCrop(t.config.Width, t.config.Height, img, 2)
						return
					}
				}
			}
		}
	}
	// auto generate
	tmpH := 6
	tmpW := int(float64(t.config.Width) / float64(t.config.Height) * float64(tmpH))
	bgColor := t.RandDeepColor(0, titleSum)
	m := image.NewRGBA(image.Rect(0, 0, tmpW, tmpH))

	draw.Draw(m, m.Bounds(), &image.Uniform{C: bgColor}, image.Point{}, draw.Src)
	if t.config.Noise {
		n := 0
		for i := 0; i < tmpH; i++ {
			if titleSum[i]%2 != 0 {
				continue
			}
			for j := 0; j < tmpW; j++ {
				n = (n + 1) % 32
				if titleSum[n]%3 == 0 {
					m.Set(j, i, t.RandDeepColor(int(titleSum[n])%22, titleSum))
				}
			}
		}
	}
	newImg = imaging.Resize(m, t.config.Width, t.config.Height, imaging.Gaussian)
	return
}

func (t *TitleImage) Save(w *Website, img image.Image, title string) (string, error) {
	imgType := "png"
	if t.useWebp == 1 {
		imgType = "webp"
	}

	buf, _ := encodeImage(img, imgType, 100)

	fileHeader := &multipart.FileHeader{
		Filename: library.Md5(title) + "." + imgType,
		Header:   nil,
		Size:     int64(len(buf)),
	}

	tmpfile, _ := os.CreateTemp("", fileHeader.Filename)
	defer os.Remove(tmpfile.Name()) // clean up
	tmpfile.Write(buf)

	attachment, err := w.AttachmentUpload(tmpfile, fileHeader, 0, 0)
	if err != nil {
		return "", err
	}

	return attachment.Logo, nil
}

func (t *TitleImage) EncodeB64string(img image.Image) string {
	buf, _ := encodeImage(img, "webp", 85)

	return fmt.Sprintf("data:%s;base64,%s", "image/webp", base64.StdEncoding.EncodeToString(buf))
}

// drawTitle 采用分词方式优化文字排版
func (t *TitleImage) drawTitle(img image.Image, title string) image.Image {
	c := freetype.NewContext()
	c.SetDPI(72)
	c.SetClip(img.Bounds())
	m := image.NewNRGBA(img.Bounds())
	draw.Draw(m, img.Bounds(), img, image.Point{}, draw.Src)
	c.SetDst(m)
	c.SetHinting(font.HintingFull)
	c.SetFont(t.font)
	// 文字最小，最大
	minSize := t.fontSize
	maxSize := 100
	gap := 100
	maxTextWidth := t.config.Width - gap
	realSize := maxTextWidth / utf8.RuneCountInString(title)
	if realSize < minSize {
		realSize = minSize
	} else if realSize > maxSize {
		realSize = maxSize
	}
	c.SetFontSize(float64(realSize))
	c.SetSrc(image.NewUniform(library.HEXToRGB(t.config.FontColor)))

	words := library.WordSplit(title, true)
	var lineWords []string
	var tmpWords string
	var tmpWidth int
	for i, v := range words {
		vWidth := t.getLettersLen([]rune(v), realSize)
		tmpWidth += vWidth
		if tmpWidth <= maxTextWidth {
			tmpWords += v
		}
		if tmpWidth >= maxTextWidth {
			lineWords = append(lineWords, tmpWords)
			if tmpWidth > maxTextWidth {
				tmpWidth = vWidth
				tmpWords = v
			} else {
				tmpWords = ""
				tmpWidth = 0
			}
		}
		if i == len(words)-1 && len(tmpWords) > 0 {
			lineWords = append(lineWords, tmpWords)
		}
	}

	lineLen := len(lineWords)
	// 行高 size * 1.6
	startY := t.config.Height/2 - (int(float64(realSize)/1.5) + int(float64((lineLen-1)*realSize)*1.6)/2) + realSize
	for i, tmpText := range lineWords {
		tmpWidth = t.getLettersLen([]rune(tmpText), realSize)
		startX := (t.config.Width - tmpWidth) / 2
		if i > 0 {
			startY += int(float64(realSize) * 1.6)
		}
		pt := freetype.Pt(startX, startY)
		_, _ = c.DrawString(tmpText, pt)
	}

	return m
}

// countLetter 计算字体宽度
func (t *TitleImage) getLettersLen(ss []rune, fontSize int) int {
	var width int
	for _, s := range ss {
		hm := t.font.HMetric(fixed.Int26_6(fontSize), t.font.Index(s))
		width += int(hm.AdvanceWidth)
	}
	return width
}

func (t *TitleImage) RandDeepColor(addon int, titleSum string) color.RGBA {
	randColor := t.RandColor(addon, titleSum)
	num, _ := strconv.Atoi(titleSum[22-addon : 22-addon+9])
	increase := float64(30 + num%255)

	red := math.Abs(math.Min(float64(randColor.R)-increase, 255))

	green := math.Abs(math.Min(float64(randColor.G)-increase, 255))
	blue := math.Abs(math.Min(float64(randColor.B)-increase, 255))

	return color.RGBA{R: uint8(red), G: uint8(green), B: uint8(blue), A: uint8(255)}
}

// RandColor get random color. 生成随机颜色.
func (t *TitleImage) RandColor(addon int, titleSum string) color.RGBA {
	num, _ := strconv.Atoi(titleSum[addon : addon+9])
	red := num % 255
	green := num / 1000 % 255
	var blue int
	if (red + green) > 400 {
		blue = 0
	} else {
		blue = 400 - green - red
	}
	if blue > 255 {
		blue = 255
	}
	return color.RGBA{R: uint8(red), G: uint8(green), B: uint8(blue), A: uint8(255)}
}

func loadLocalFont(diyPath string) *truetype.Font {
	// if exist diy font file, then use diy font file
	info, err := os.Stat(diyPath)
	if err == nil && !info.IsDir() {
		phtf, err := os.ReadFile(diyPath)
		phtft, err := freetype.ParseFont(phtf)
		if err == nil {
			return phtft
		}
	}

	fontPaths := findfont.List()
	for _, path := range fontPaths {
		//楷体:simkai.ttf
		//黑体:simhei.ttf
		if strings.Contains(path, "yahei") ||
			strings.Contains(path, "simhei") ||
			strings.Contains(path, "simkai.ttf") ||
			strings.Contains(path, "PingFang.ttc") ||
			strings.Contains(path, "Heiti") ||
			strings.Contains(path, "simsun.ttc") {
			phtf, err := os.ReadFile(path)
			phtft, err := freetype.ParseFont(phtf)
			if err != nil {
				continue
			}

			return phtft
		}
	}
	for _, path := range fontPaths {
		info, err = os.Stat(path)
		if err == nil {
			if info.Size() > 1024*1024*2 {
				phtf, err := os.ReadFile(path)
				phtft, err := freetype.ParseFont(phtf)
				if err != nil {
					continue
				}

				return phtft
			}
		}
	}
	// 英文状态下的默认字体
	for _, path := range fontPaths {
		// 选择解析成功的第一个
		phtf, err := os.ReadFile(path)
		phtft, err := freetype.ParseFont(phtf)
		if err != nil {
			continue
		}

		return phtft
	}

	return nil
}
