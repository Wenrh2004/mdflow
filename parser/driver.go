package parser

import (
	"github.com/Wenrh2004/mdflow/internal/drive"
	"github.com/Wenrh2004/mdflow/token"
)

// The facade's side of the block machine. BlockState's driver operations are
// unexported so they stay out of the extension API; blockDriver exposes them
// to package drive, which the mdflow facade imports. blockDriver shares
// BlockState's memory layout, so converting between them is free.

func init() {
	drive.NewBlock = func(rules any) drive.Block {
		return (*blockDriver)(newBlockState(rules.(*RuleSet)))
	}
}

func eachLine(src string, fn func(line string) bool) { drive.EachLine(src, fn) }

type blockDriver BlockState

var (
	_ drive.Block  = (*blockDriver)(nil)
	_ drive.Cursor = (*inlineCursor)(nil)
)

func (d *blockDriver) state() *BlockState { return (*BlockState)(d) }

func (d *blockDriver) FeedLine(line string) []token.BlockEvent { return d.state().feedLine(line) }
func (d *blockDriver) CloseAll() []token.BlockEvent            { return d.state().closeAll() }
func (d *blockDriver) ReleaseEvents()                          { d.state().releaseEvents() }
func (d *blockDriver) CollectAll(src string) []token.BlockEvent {
	return d.state().collectAll(src)
}
func (d *blockDriver) SealReferences()              { d.state().sealReferences() }
func (d *blockDriver) ReferencesRefused() bool      { return d.state().referencesRefused() }
func (d *blockDriver) HeldCount() int               { return d.state().heldCount() }
func (d *blockDriver) ReferenceFingerprint() uint64 { return d.state().referenceFingerprint() }
func (d *blockDriver) Total() int                   { return d.state().total() }
func (d *blockDriver) Clone() drive.Block           { return (*blockDriver)(d.state().clone()) }
func (d *blockDriver) Reset()                       { d.state().reset(d.rules) }
func (d *blockDriver) ParseInlineFinal(leaf token.Leaf) []token.Inline {
	return d.state().parseInlineFinal(leaf)
}

func (d *blockDriver) AppendInline(dst []token.Inline, leaf token.Leaf) ([]token.Inline, drive.Cursor) {
	tokens, cursor := d.state().appendInline(dst, leaf)
	if cursor == nil {
		return tokens, nil // a typed nil would read as a paused cursor
	}
	return tokens, cursor
}

// CloneFor implements drive.Cursor; the Block is always a blockDriver.
func (c *inlineCursor) CloneFor(b drive.Block) drive.Cursor {
	var block *BlockState
	if b != nil {
		block = b.(*blockDriver).state()
	}
	if cloned := c.cloneFor(block); cloned != nil {
		return cloned
	}
	return nil
}
