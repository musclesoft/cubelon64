package audiotrace

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"cubelon-emu/sidemu"
)

const ayClockHz = 1773447.0

type Channel struct {
	Hz    float64
	Level float64
}

type Row struct {
	Channels [3]Channel
	Frame    int
}

func AYRow(frame int, registers [16]byte, envelopeLevel int) Row {
	row := Row{Frame: frame}
	mixer := registers[7]
	for channel := 0; channel < 3; channel++ {
		toneOn := mixer>>channel&1 == 0
		noiseOn := mixer>>(channel+3)&1 == 0
		if !toneOn && !noiseOn {
			continue
		}
		volume := registers[8+channel]
		level := float64(volume & 0x0F)
		if volume&0x10 != 0 {
			level = float64(envelopeLevel)
		}
		row.Channels[channel].Level = level / 15
		if toneOn {
			period := int(registers[channel*2]) | int(registers[channel*2+1]&0x0F)<<8
			row.Channels[channel].Hz = ayClockHz / (16 * float64(max(period, 1)))
		}
	}
	return row
}

func SIDRow(frame int, sid *sidemu.Emulator) Row {
	row := Row{Frame: frame}
	masterVolume := float64(sid.Registers[0x18]&0x0F) / 15
	for voice := 0; voice < 3; voice++ {
		state := sid.Voices[voice]
		if state.Waveform == 0 {
			continue
		}
		row.Channels[voice].Level = float64(state.EnvelopeLevel) / 255 * masterVolume
		if state.Waveform != 8 {
			row.Channels[voice].Hz = state.Frequency
		}
	}
	return row
}

func WriteCSV(path string, rows []Row) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	fmt.Fprintln(writer, "frame,a_level,a_hz,b_level,b_hz,c_level,c_hz")
	for _, row := range rows {
		fmt.Fprintf(writer, "%d", row.Frame)
		for _, channel := range row.Channels {
			fmt.Fprintf(writer, ",%.4f,%.2f", channel.Level, channel.Hz)
		}
		fmt.Fprintln(writer)
	}
	return writer.Flush()
}

func ReadCSV(path string) ([]Row, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	rows := make([]Row, 0, len(lines))
	for _, line := range lines[1:] {
		fields := strings.Split(line, ",")
		if len(fields) != 7 {
			return nil, fmt.Errorf("bad row %q in %s", line, path)
		}
		frame, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, err
		}
		row := Row{Frame: frame}
		for channel := 0; channel < 3; channel++ {
			level, err := strconv.ParseFloat(fields[1+channel*2], 64)
			if err != nil {
				return nil, err
			}
			hz, err := strconv.ParseFloat(fields[2+channel*2], 64)
			if err != nil {
				return nil, err
			}
			row.Channels[channel] = Channel{Hz: hz, Level: level}
		}
		rows = append(rows, row)
	}
	return rows, nil
}
