package main

import "sort"

type blockPhrase struct {
	seq string
}

// streamItem is either a literal stream byte or a reference to a selected block.
type streamItem struct {
	block   int
	literal byte
}

const (
	maxBlockPhraseLength = 64
	blockLengthOverhead  = 2 // body length byte and block length table byte
	literalItem          = -1
)

type phraseOccurrence struct {
	run    int
	offset int
}

// selectBlocksGreedy repeatedly takes the phrase with the largest saving over all
// channel streams. Occurrences already covered by a chosen block split their stream
// into runs of literals, so later phrases never contain a block reference.
func selectBlocksGreedy(streams [3][]byte) ([]blockPhrase, [3][]streamItem) {
	var parsed [3][]streamItem
	for channel, stream := range streams {
		for _, value := range stream {
			parsed[channel] = append(parsed[channel], streamItem{block: literalItem, literal: value})
		}
	}
	var phrases []blockPhrase
	for {
		var runs [][]byte
		var runChannel []int
		var runStart []int
		for channel := range parsed {
			var run []byte
			start := 0
			flush := func(end int) {
				if len(run) > 0 {
					runs = append(runs, run)
					runChannel = append(runChannel, channel)
					runStart = append(runStart, start)
				}
				run = nil
				start = end + 1
			}
			for index, item := range parsed[channel] {
				if item.block != literalItem {
					flush(index)
					continue
				}
				run = append(run, item.literal)
			}
			flush(len(parsed[channel]))
		}
		occurrences := make(map[string][]phraseOccurrence)
		for runIndex, run := range runs {
			for start := range run {
				for length := 2; length <= maxBlockPhraseLength && start+length <= len(run); length++ {
					phrase := string(run[start : start+length])
					occurrences[phrase] = append(occurrences[phrase], phraseOccurrence{runIndex, start})
				}
			}
		}
		bestSaving := 0
		var best string
		var bestOccurrences []phraseOccurrence
		for phrase, all := range occurrences {
			if len(all) < 2 {
				continue
			}
			var apart []phraseOccurrence
			runEnd := -1
			lastRun := -1
			for _, occurrence := range all {
				if occurrence.run != lastRun {
					lastRun = occurrence.run
					runEnd = -1
				}
				if occurrence.offset >= runEnd {
					apart = append(apart, occurrence)
					runEnd = occurrence.offset + len(phrase)
				}
			}
			saving := len(apart)*(len(phrase)-1) - len(phrase) - blockLengthOverhead
			if saving > bestSaving || saving == bestSaving && saving > 0 && occursBefore(all[0], len(phrase), bestOccurrences[0], len(best)) {
				bestSaving = saving
				best = phrase
				bestOccurrences = apart
			}
		}
		if bestSaving <= 0 {
			break
		}
		blockIndex := len(phrases)
		phrases = append(phrases, blockPhrase{seq: best})
		sort.Slice(bestOccurrences, func(a, b int) bool {
			if bestOccurrences[a].run != bestOccurrences[b].run {
				return bestOccurrences[a].run < bestOccurrences[b].run
			}
			return bestOccurrences[a].offset < bestOccurrences[b].offset
		})
		replacements := make(map[int][]int)
		for _, occurrence := range bestOccurrences {
			channel := runChannel[occurrence.run]
			replacements[channel] = append(replacements[channel], runStart[occurrence.run]+occurrence.offset)
		}
		for channel, starts := range replacements {
			var next []streamItem
			position := 0
			for _, start := range starts {
				next = append(next, parsed[channel][position:start]...)
				next = append(next, streamItem{block: blockIndex})
				position = start + len(best)
			}
			next = append(next, parsed[channel][position:]...)
			parsed[channel] = next
		}
	}
	return phrases, parsed
}

// occursBefore orders equal-saving phrases by where they first occur, shorter first.
func occursBefore(a phraseOccurrence, aLength int, b phraseOccurrence, bLength int) bool {
	if a.run != b.run {
		return a.run < b.run
	}
	if a.offset != b.offset {
		return a.offset < b.offset
	}
	return aLength < bLength
}
