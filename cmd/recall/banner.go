package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Mystical portal styles using shared brand colors from styles.go
	bannerDimStyle     = lipgloss.NewStyle().Foreground(colorMuted)
	bannerStarStyle    = lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	bannerSparkStyle   = lipgloss.NewStyle().Foreground(colorPrimaryLight)
	bannerTitleStyle   = lipgloss.NewStyle().Foreground(colorText).Bold(true)
	bannerTaglineStyle = lipgloss.NewStyle().Foreground(colorPrimaryDark).Italic(true)
	bannerVersionStyle = lipgloss.NewStyle().Foreground(colorMuted)
)

func renderBanner() string {
	// Build styled characters
	dot := bannerDimStyle.Render("·")
	period := bannerDimStyle.Render(".")
	apos := bannerDimStyle.Render("'")
	star := bannerStarStyle.Render("✦")
	spark := bannerSparkStyle.Render("✧")
	title := bannerTitleStyle.Render("RECALL")

	// Construct the portal as a slice for clarity
	lines := []string{
		"        " + period + " " + dot + " " + star + " " + dot + " " + period,
		"      " + dot + "   " + spark + " " + dot + " " + spark + "   " + dot,
		"    " + dot + "    " + title + "    " + dot,
		"      " + dot + "   " + spark + " " + dot + " " + spark + "   " + dot,
		"        " + apos + " " + dot + " " + star + " " + dot + " " + apos,
	}

	return strings.Join(lines, "\n")
}

func renderBannerWithTagline() string {
	banner := renderBanner()
	tagline := bannerTaglineStyle.Render("    echoes of experience")
	ver := bannerVersionStyle.Render("           " + version)

	return strings.Join([]string{banner, tagline, ver}, "\n")
}
