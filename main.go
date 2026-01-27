package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/patrickjmcd/hdhr2xmltv/internal/hdhr"
	"github.com/patrickjmcd/hdhr2xmltv/internal/xmltv"
)

var (
	version = "dev"
)

func main() {
	deviceIP := flag.String("ip", "192.168.1.100", "HDHomeRun device IP address")
	outputFile := flag.String("output", "", "Output file (default: stdout)")
	showVersion := flag.Bool("version", false, "Show version information")
	verbose := flag.Bool("verbose", false, "Enable verbose output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "hdhr2xmltv - Fetch TV guide from HDHomeRun and output in XMLTV format\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -ip 192.168.1.100\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -ip 192.168.1.100 -output guide.xml\n", os.Args[0])
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("hdhr2xmltv version %s\n", version)
		os.Exit(0)
	}

	if err := run(*deviceIP, *outputFile, *verbose); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(deviceIP, outputFile string, verbose bool) error {
	client := hdhr.NewClient(deviceIP)

	if verbose {
		fmt.Fprintf(os.Stderr, "Connecting to HDHomeRun at %s...\n", deviceIP)
	}

	deviceInfo, err := client.GetDeviceInfo()
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Connected to %s (%s)\n", deviceInfo.FriendlyName, deviceInfo.ModelNumber)
		fmt.Fprintf(os.Stderr, "Device ID: %s\n", deviceInfo.DeviceID)
		fmt.Fprintf(os.Stderr, "Firmware: %s %s\n", deviceInfo.FirmwareName, deviceInfo.FirmwareVersion)
		fmt.Fprintf(os.Stderr, "LineupURL: %s\n", deviceInfo.LineupURL)
	}

	if deviceInfo.DeviceAuth == "" {
		return fmt.Errorf("device did not provide DeviceAuth - guide data may not be available for this device")
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "DeviceAuth: %s\n", deviceInfo.DeviceAuth)
		fmt.Fprintf(os.Stderr, "Fetching TV guide data...\n")
	}

	guide, err := client.GetGuide(deviceInfo)
	if err != nil {
		return fmt.Errorf("failed to get guide: %w", err)
	}

	if verbose {
		programCount := 0
		for _, ch := range guide {
			programCount += len(ch.Guide)
		}
		fmt.Fprintf(os.Stderr, "Retrieved %d channels with %d programs\n", len(guide), programCount)
	}

	var output *os.File
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
		defer f.Close()
		output = f
	} else {
		output = os.Stdout
	}

	writer := xmltv.NewWriter(output)
	if err := writer.Write(guide); err != nil {
		return fmt.Errorf("failed to write XMLTV: %w", err)
	}

	if verbose && outputFile != "" {
		fmt.Fprintf(os.Stderr, "XMLTV guide written to %s\n", outputFile)
	}

	return nil
}
