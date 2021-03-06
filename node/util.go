package node

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/absolute8511/redcon"
	"github.com/youzan/ZanRedisDB/common"
)

var nodeLog = common.NewLevelLogger(common.LOG_INFO, common.NewDefaultLogger("node"))
var syncerOnly int32
var syncerOnlyChangedTs int64

func SetLogLevel(level int) {
	nodeLog.SetLevel(int32(level))
}

func SetLogger(level int32, logger common.Logger) {
	nodeLog.SetLevel(level)
	nodeLog.Logger = logger
}

func SetSyncerOnly(enable bool) {
	if enable {
		atomic.StoreInt32(&syncerOnly, 1)
	} else {
		atomic.StoreInt32(&syncerOnly, 0)
		atomic.StoreInt64(&syncerOnlyChangedTs, time.Now().UnixNano())
	}
}

func GetSyncedOnlyChangedTs() int64 {
	return atomic.LoadInt64(&syncerOnlyChangedTs)
}

func IsSyncerOnly() bool {
	return atomic.LoadInt32(&syncerOnly) == 1
}

func checkOKRsp(cmd redcon.Command, v interface{}) (interface{}, error) {
	return "OK", nil
}

func checkAndRewriteIntRsp(cmd redcon.Command, v interface{}) (interface{}, error) {
	if rsp, ok := v.(int64); ok {
		return rsp, nil
	}
	return nil, errInvalidResponse
}

func checkAndRewriteBulkRsp(cmd redcon.Command, v interface{}) (interface{}, error) {
	if v == nil {
		return nil, nil
	}
	rsp, ok := v.([]byte)
	if ok {
		return rsp, nil
	}
	return nil, errInvalidResponse
}

func buildCommand(args [][]byte) redcon.Command {
	return common.BuildCommand(args)
}

// we can only use redis v2 for single key write command, otherwise we need cut namespace for different keys in different command
func rebuildFirstKeyAndPropose(kvn *KVNode, cmd redcon.Command, f common.CommandRspFunc) (redcon.Command,
	interface{}, error) {

	var rsp *FutureRsp
	var err error
	if !UseRedisV2 {
		key, err := common.CutNamesapce(cmd.Args[1])
		if err != nil {
			return cmd, nil, err
		}

		cmd.Args[1] = key
		ncmd := buildCommand(cmd.Args)
		copy(cmd.Raw[0:], ncmd.Raw[:])
		cmd.Raw = cmd.Raw[:len(ncmd.Raw)]
		rsp, err = kvn.RedisProposeAsync(cmd.Raw)
	} else {
		rsp, err = kvn.RedisV2ProposeAsync(cmd.Raw)
	}
	if err != nil {
		return cmd, nil, err
	}
	if f != nil {
		rsp.rspHandle = func(r interface{}) (interface{}, error) {
			return f(cmd, r)
		}
	}
	return cmd, rsp, err
}

func wrapReadCommandK(f common.CommandFunc) common.CommandFunc {
	return func(conn redcon.Conn, cmd redcon.Command) {
		if len(cmd.Args) != 2 {
			conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
			return
		}
		key, err := common.CutNamesapce(cmd.Args[1])
		if err != nil {
			conn.WriteError(err.Error())
			return
		}
		cmd.Args[1] = key
		f(conn, cmd)
	}
}

func wrapReadCommandKSubkey(f common.CommandFunc) common.CommandFunc {
	return func(conn redcon.Conn, cmd redcon.Command) {
		if len(cmd.Args) != 3 {
			conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
			return
		}
		key, err := common.CutNamesapce(cmd.Args[1])
		if err != nil {
			conn.WriteError(err.Error())
			return
		}
		cmd.Args[1] = key
		f(conn, cmd)
	}
}

func wrapReadCommandKSubkeySubkey(f common.CommandFunc) common.CommandFunc {
	return func(conn redcon.Conn, cmd redcon.Command) {
		if len(cmd.Args) < 3 {
			conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
			return
		}
		key, err := common.CutNamesapce(cmd.Args[1])
		if err != nil {
			conn.WriteError(err.Error())
			return
		}
		cmd.Args[1] = key
		f(conn, cmd)
	}
}

func wrapReadCommandKAnySubkey(f common.CommandFunc) common.CommandFunc {
	return wrapReadCommandKAnySubkeyN(f, 0)
}

func wrapReadCommandKAnySubkeyN(f common.CommandFunc, minSubLen int) common.CommandFunc {
	return func(conn redcon.Conn, cmd redcon.Command) {
		if len(cmd.Args) < 2+minSubLen {
			conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
			return
		}
		key, err := common.CutNamesapce(cmd.Args[1])
		if err != nil {
			conn.WriteError(err.Error())
			return
		}
		cmd.Args[1] = key
		f(conn, cmd)
	}
}

func wrapReadCommandKK(f common.CommandFunc) common.CommandFunc {
	return func(conn redcon.Conn, cmd redcon.Command) {
		if len(cmd.Args) < 2 {
			conn.WriteError("ERR wrong number of arguments for '" + string(cmd.Args[0]) + "' command")
			return
		}
		if len(cmd.Args[1:]) > common.MAX_BATCH_NUM {
			conn.WriteError(errTooMuchBatchSize.Error())
			return
		}
		for i := 1; i < len(cmd.Args); i++ {
			key, err := common.CutNamesapce(cmd.Args[i])
			if err != nil {
				conn.WriteError(err.Error())
				return
			}
			cmd.Args[i] = key
		}
		f(conn, cmd)
	}
}

func wrapWriteCommandK(kvn *KVNode, f common.CommandRspFunc) common.WriteCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) != 2 {
			err := fmt.Errorf("ERR wrong number arguments for '%v' command", string(cmd.Args[0]))
			return nil, err
		}
		_, rsp, err := rebuildFirstKeyAndPropose(kvn, cmd, f)
		return rsp, err
	}
}

/*
func wrapWriteCommandKK(kvn *KVNode, f common.CommandRspFunc) common.WriteCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) < 2 {
			err := fmt.Errorf("ERR wrong number arguments for '%v' command", string(cmd.Args[0]))
			return nil, err
		}
		args := cmd.Args[1:]
		if len(args) > common.MAX_BATCH_NUM {
			return nil, errTooMuchBatchSize
		}
		for i, v := range args {
			key, err := common.CutNamesapce(v)
			if err != nil {
				return nil, err
			}

			args[i] = key
		}
		ncmd := buildCommand(cmd.Args)
		copy(cmd.Raw[0:], ncmd.Raw[:])
		cmd.Raw = cmd.Raw[:len(ncmd.Raw)]

		rsp, err := kvn.RedisProposeAsync(cmd.Raw)
		if err != nil {
			return nil, err
		}
		if f != nil {
			rsp.rspHandle = func(r interface{}) (interface{}, error) {
				return f(cmd, r)
			}
		}
		return rsp, nil
	}
}
*/

func wrapWriteCommandKSubkey(kvn *KVNode, f common.CommandRspFunc) common.WriteCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) != 3 {
			err := fmt.Errorf("ERR wrong number arguments for '%v' command", string(cmd.Args[0]))
			return nil, err
		}
		_, rsp, err := rebuildFirstKeyAndPropose(kvn, cmd, f)
		return rsp, err
	}
}

func wrapWriteCommandKSubkeySubkey(kvn *KVNode, f common.CommandRspFunc) common.WriteCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) < 3 {
			err := fmt.Errorf("ERR wrong number arguments for '%v' command", string(cmd.Args[0]))
			return nil, err
		}
		_, rsp, err := rebuildFirstKeyAndPropose(kvn, cmd, f)
		return rsp, err
	}
}

func wrapWriteCommandKAnySubkey(kvn *KVNode, f common.CommandRspFunc, minSubKeyLen int) common.WriteCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) < 2+minSubKeyLen {
			err := fmt.Errorf("ERR wrong number arguments for '%v' command", string(cmd.Args[0]))
			return nil, err
		}
		_, rsp, err := rebuildFirstKeyAndPropose(kvn, cmd, f)
		return rsp, err
	}
}

func wrapWriteCommandKV(kvn *KVNode, f common.CommandRspFunc) common.WriteCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) != 3 {
			err := fmt.Errorf("ERR wrong number arguments for '%v' command", string(cmd.Args[0]))
			return nil, err
		}
		_, rsp, err := rebuildFirstKeyAndPropose(kvn, cmd, f)
		return rsp, err
	}
}

func wrapWriteCommandKVV(kvn *KVNode, f common.CommandRspFunc) common.WriteCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) < 3 {
			err := fmt.Errorf("ERR wrong number arguments for '%v' command", string(cmd.Args[0]))
			return nil, err
		}
		cmd, rsp, err := rebuildFirstKeyAndPropose(kvn, cmd, f)
		return rsp, err
	}
}

/*
func wrapWriteCommandKVKV(kvn *KVNode, f common.CommandRspFunc) common.WriteCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) < 3 || len(cmd.Args[1:])%2 != 0 {
			err := fmt.Errorf("ERR wrong number arguments for '%v' command", string(cmd.Args[0]))
			return nil, err
		}
		if len(cmd.Args[1:])/2 > common.MAX_BATCH_NUM {
			return nil, errTooMuchBatchSize
		}
		args := cmd.Args[1:]
		for i, v := range args {
			if i%2 != 0 {
				continue
			}
			key, err := common.CutNamesapce(v)
			if err != nil {
				return nil, err
			}

			args[i] = key
		}
		ncmd := buildCommand(cmd.Args)
		copy(cmd.Raw[0:], ncmd.Raw[:])
		cmd.Raw = cmd.Raw[:len(ncmd.Raw)]

		rsp, err := kvn.RedisProposeAsync(cmd.Raw)
		if err != nil {
			return nil, err
		}
		if f != nil {
			rsp.rspHandle = func(r interface{}) (interface{}, error) {
				return f(cmd, r)
			}
		}
		return rsp, nil
	}
}
*/

func wrapWriteCommandKSubkeyV(kvn *KVNode, f common.CommandRspFunc) common.WriteCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) != 4 {
			err := fmt.Errorf("ERR wrong number arguments for '%v' command", string(cmd.Args[0]))
			return nil, err
		}
		_, rsp, err := rebuildFirstKeyAndPropose(kvn, cmd, f)
		return rsp, err
	}
}

func wrapWriteCommandKSubkeyVSubkeyV(kvn *KVNode, f common.CommandRspFunc) common.WriteCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) < 4 || len(cmd.Args[2:])%2 != 0 {
			err := fmt.Errorf("ERR wrong number arguments for '%v' command", string(cmd.Args[0]))
			return nil, err
		}
		if len(cmd.Args[2:])/2 > common.MAX_BATCH_NUM {
			return nil, errTooMuchBatchSize
		}
		_, rsp, err := rebuildFirstKeyAndPropose(kvn, cmd, f)
		return rsp, err
	}
}

func wrapMergeCommand(f common.MergeCommandFunc) common.MergeCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		key, err := common.CutNamesapce(cmd.Args[1])
		if err != nil {
			return nil, err
		}
		cmd.Args[1] = key

		return f(cmd)
	}
}

func wrapMergeCommandKK(f common.MergeCommandFunc) common.MergeCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) < 2 {
			return nil, fmt.Errorf("ERR wrong number of arguments for '%s' command", string(cmd.Args[0]))
		}
		if len(cmd.Args[1:]) > common.MAX_BATCH_NUM {
			return nil, errTooMuchBatchSize
		}
		for i := 1; i < len(cmd.Args); i++ {
			key, err := common.CutNamesapce(cmd.Args[i])
			if err != nil {
				return nil, err
			}
			cmd.Args[i] = key
		}
		return f(cmd)
	}
}

func wrapWriteMergeCommandKK(kvn *KVNode, f common.MergeWriteCommandFunc) common.MergeCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) < 2 {
			return nil, fmt.Errorf("ERR wrong number of arguments for '%s' command", string(cmd.Args[0]))
		}
		args := cmd.Args[1:]
		if len(args) > common.MAX_BATCH_NUM {
			return nil, errTooMuchBatchSize
		}
		for i, v := range args {
			key, err := common.CutNamesapce(v)
			if err != nil {
				return nil, err
			}
			args[i] = key
		}
		ncmd := buildCommand(cmd.Args)
		copy(cmd.Raw[0:], ncmd.Raw[:])
		cmd.Raw = cmd.Raw[:len(ncmd.Raw)]

		rsp, err := kvn.RedisPropose(cmd.Raw)
		if err != nil {
			return nil, err
		}
		if f != nil {
			return f(cmd, rsp)
		}
		return rsp, nil
	}
}

func wrapWriteMergeCommandKVKV(kvn *KVNode, f common.MergeWriteCommandFunc) common.MergeCommandFunc {
	return func(cmd redcon.Command) (interface{}, error) {
		if len(cmd.Args) < 3 || len(cmd.Args[1:])%2 != 0 {
			return nil, fmt.Errorf("ERR wrong number arguments for '%s' command", string(cmd.Args[0]))
		}
		if len(cmd.Args[1:])/2 > common.MAX_BATCH_NUM {
			return nil, errTooMuchBatchSize
		}
		args := cmd.Args[1:]
		for i, v := range args {
			if i%2 != 0 {
				continue
			}
			key, err := common.CutNamesapce(v)
			if err != nil {
				return nil, err
			}
			args[i] = key
		}
		ncmd := buildCommand(cmd.Args)
		copy(cmd.Raw[0:], ncmd.Raw[:])
		cmd.Raw = cmd.Raw[:len(ncmd.Raw)]

		rsp, err := kvn.RedisPropose(cmd.Raw)
		if err != nil {
			return nil, err
		}
		if f != nil {
			return f(cmd, rsp)
		}
		return rsp, nil
	}
}

type notifier struct {
	c   chan struct{}
	err error
}

func newNotifier() *notifier {
	return &notifier{
		c: make(chan struct{}),
	}
}

func (nc *notifier) notify(err error) {
	nc.err = err
	close(nc.c)
}

func CopyFileForHardLink(src, dst string) error {
	// open source file
	sfi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !sfi.Mode().IsRegular() {
		return fmt.Errorf("non-regular source file %s (%q)", sfi.Name(), sfi.Mode().String())
	}

	// open dest file
	dfi, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		// file doesn't exist
		err := os.MkdirAll(filepath.Dir(dst), common.DIR_PERM)
		if err != nil {
			return err
		}
	} else {
		if !(dfi.Mode().IsRegular()) {
			return fmt.Errorf("non-regular destination file %s (%q)", dfi.Name(), dfi.Mode().String())
		}
		if os.SameFile(sfi, dfi) {
			return nil
		}
	}
	if err = os.Link(src, dst); err == nil {
		return nil
	}
	err = copyFileContents(src, dst)
	if err != nil {
		return err
	}
	os.Chmod(dst, sfi.Mode())
	return nil
}

// copyFileContents copies the contents.
// all contents will be replaced by the source .
func copyFileContents(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	// we remove dst to avoid override the hard link file content which may affect the origin linked file
	err = os.Remove(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	}
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		cerr := dstFile.Close()
		if err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	err = dstFile.Sync()
	return err
}
