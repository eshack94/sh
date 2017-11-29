// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package interp

import (
	"os"
	"os/exec"
	"regexp"

	"golang.org/x/crypto/ssh/terminal"

	"mvdan.cc/sh/syntax"
)

// non-empty string is true, empty string is false
func (r *Runner) bashTest(expr syntax.TestExpr) string {
	switch x := expr.(type) {
	case *syntax.Word:
		return r.loneWord(x)
	case *syntax.ParenTest:
		return r.bashTest(x.X)
	case *syntax.BinaryTest:
		switch x.Op {
		case syntax.TsMatch, syntax.TsNoMatch:
			str := r.loneWord(x.X.(*syntax.Word))
			yw := x.Y.(*syntax.Word)
			pat := r.lonePattern(yw)
			if match(pat, str) == (x.Op == syntax.TsMatch) {
				return "1"
			}
			return ""
		}
		if r.binTest(x.Op, r.bashTest(x.X), r.bashTest(x.Y)) {
			return "1"
		}
		return ""
	case *syntax.UnaryTest:
		if r.unTest(x.Op, r.bashTest(x.X)) {
			return "1"
		}
		return ""
	}
	return ""
}

func (r *Runner) binTest(op syntax.BinTestOperator, x, y string) bool {
	switch op {
	case syntax.TsReMatch:
		re, err := regexp.Compile(y)
		if err != nil {
			r.exit = 2
			return false
		}
		return re.MatchString(x)
	case syntax.TsNewer:
		i1, i2 := r.stat(x), r.stat(y)
		if i1 == nil || i2 == nil {
			return false
		}
		return i1.ModTime().After(i2.ModTime())
	case syntax.TsOlder:
		i1, i2 := r.stat(x), r.stat(y)
		if i1 == nil || i2 == nil {
			return false
		}
		return i1.ModTime().Before(i2.ModTime())
	case syntax.TsDevIno:
		i1, i2 := r.stat(x), r.stat(y)
		return os.SameFile(i1, i2)
	case syntax.TsEql:
		return atoi(x) == atoi(y)
	case syntax.TsNeq:
		return atoi(x) != atoi(y)
	case syntax.TsLeq:
		return atoi(x) <= atoi(y)
	case syntax.TsGeq:
		return atoi(x) >= atoi(y)
	case syntax.TsLss:
		return atoi(x) < atoi(y)
	case syntax.TsGtr:
		return atoi(x) > atoi(y)
	case syntax.AndTest:
		return x != "" && y != ""
	case syntax.OrTest:
		return x != "" || y != ""
	case syntax.TsBefore:
		return x < y
	default: // syntax.TsAfter
		return x > y
	}
}

func (r *Runner) stat(name string) os.FileInfo {
	info, _ := os.Stat(r.relPath(name))
	return info
}

func (r *Runner) statMode(name string, mode os.FileMode) bool {
	info := r.stat(name)
	return info != nil && info.Mode()&mode != 0
}

func (r *Runner) unTest(op syntax.UnTestOperator, x string) bool {
	switch op {
	case syntax.TsExists:
		return r.stat(x) != nil
	case syntax.TsRegFile:
		info := r.stat(x)
		return info != nil && info.Mode().IsRegular()
	case syntax.TsDirect:
		return r.statMode(x, os.ModeDir)
	case syntax.TsCharSp:
		return r.statMode(x, os.ModeCharDevice)
	case syntax.TsBlckSp:
		info := r.stat(x)
		return info != nil && info.Mode()&os.ModeDevice != 0 &&
			info.Mode()&os.ModeCharDevice == 0
	case syntax.TsNmPipe:
		return r.statMode(x, os.ModeNamedPipe)
	case syntax.TsSocket:
		return r.statMode(x, os.ModeSocket)
	case syntax.TsSmbLink:
		info, _ := os.Lstat(r.relPath(x))
		return info != nil && info.Mode()&os.ModeSymlink != 0
	case syntax.TsSticky:
		return r.statMode(x, os.ModeSticky)
	case syntax.TsUIDSet:
		return r.statMode(x, os.ModeSetuid)
	case syntax.TsGIDSet:
		return r.statMode(x, os.ModeSetgid)
	//case syntax.TsGrpOwn:
	//case syntax.TsUsrOwn:
	//case syntax.TsModif:
	case syntax.TsRead:
		f, err := r.open(r.relPath(x), os.O_RDONLY, 0, false)
		if err == nil {
			f.Close()
		}
		return err == nil
	case syntax.TsWrite:
		f, err := r.open(r.relPath(x), os.O_WRONLY, 0, false)
		if err == nil {
			f.Close()
		}
		return err == nil
	case syntax.TsExec:
		_, err := exec.LookPath(r.relPath(x))
		return err == nil
	case syntax.TsNoEmpty:
		info := r.stat(x)
		return info != nil && info.Size() > 0
	case syntax.TsFdTerm:
		return terminal.IsTerminal(atoi(x))
	case syntax.TsEmpStr:
		return x == ""
	case syntax.TsNempStr:
		return x != ""
	case syntax.TsOptSet:
		switch x {
		case "errexit":
			return r.stopOnCmdErr
		default:
			return false
		}
	case syntax.TsVarSet:
		_, e := r.lookupVar(x)
		return e
	case syntax.TsRefVar:
		v, _ := r.lookupVar(x)
		_, ok := v.(nameRef)
		return ok
	case syntax.TsNot:
		return x == ""
	default:
		r.runErr(syntax.Pos{}, "unhandled unary test op: %v", op)
		return false
	}
}
