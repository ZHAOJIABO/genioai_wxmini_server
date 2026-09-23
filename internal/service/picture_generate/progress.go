package picture_generate

import (
	"context"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"

	"va_visionai_server/internal/constants"
	"va_visionai_server/internal/dao"
	"va_visionai_server/internal/model"
	"va_visionai_server/internal/zlog"
)

// 供不上报进度的通道（ComfyUI / Sora2 / Minimax video / Kling）使用：等待出图期间没有真实进度，只能模拟。
// 走 AIGC Core 的执行器改用 mapAigcProgress 直接采用上报值。
// 每次轮询推进"剩余距离"的固定比例，在轮询间隔固定时等价于按时间指数缓出：
// 前期涨得快、越接近上限越慢，快任务在厂商返回前也能爬到 60~75，避免尾部一次跳几十个点。
// pace 与 ProviderStatusSync 的轮询间隔（DefaultProviderSyncPollInterval, 2s）绑定，
// 改间隔时要按 newPace = 1 - (1-oldPace)^(newInterval/oldInterval) 同步调整，否则整体快慢会变。
const (
	simulatedPaceImage = 0.08
	simulatedPaceVideo = 0.025
)

// 下载/上传阶段的模拟进度上限，留出 ProgressFinalizing(95) 和 ProgressCompleted(100) 两档给真实节点。
const (
	tailRampMax      = 94
	tailRampInterval = 700 * time.Millisecond
)

// AIGC Core 上报到 aigcGenerationDoneProgress 时生图已完成，之后是我们自己的下载/上传阶段。
const aigcGenerationDoneProgress = 60

// mapAigcProgress 把 AIGC Core 上报的 0~60 线性拉伸到我们的 15~ProgressProviderMaxSimulated(80)，
// 使生图完成正好落在 80，再由 ProgressDownloading(85)+tailRamp 接手，全程连续无跳变。
// 超过 60 的上报值一律钳到 80，交给下载阶段处理。
func mapAigcProgress(aigcProgress int32) int32 {
	const floor = int32(15)
	ceiling := int32(constants.ProgressProviderMaxSimulated)
	if aigcProgress <= 0 {
		return floor
	}
	if aigcProgress >= aigcGenerationDoneProgress {
		return ceiling
	}
	mapped := floor + int32(math.Round(float64(aigcProgress)/float64(aigcGenerationDoneProgress)*float64(ceiling-floor)))
	if mapped > ceiling {
		return ceiling
	}
	return mapped
}

// nextSimulatedProgress 返回下一次轮询应展示的模拟进度，上限为 ProgressProviderMaxSimulated。
func nextSimulatedProgress(current int32, pace float64) int32 {
	ceiling := int32(constants.ProgressProviderMaxSimulated)
	if current >= ceiling {
		return current
	}

	step := int32(math.Round(float64(ceiling-current) * pace))
	if step < 1 {
		step = 1
	}
	if next := current + step; next < ceiling {
		return next
	}
	return ceiling
}

// tailRamp 在下载、上传这类拿不到细粒度进度的阶段，按固定节奏把进度往 tailRampMax 推，
// 让耗时几秒的收尾阶段不再表现为一次性跳变。
type tailRamp struct {
	stop func()
}

// startTailRamp 基于 task 的快照启动后台推进，调用方需在收尾阶段结束后 Stop。
// 快照避免与主流程并发读写同一个 task；写入走列级条件更新，不会覆盖主流程的字段或回退进度。
func startTailRamp(taskDao *dao.PictureTaskDao, task *model.PictureTask) *tailRamp {
	snapshot := *task
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(tailRampInterval)
		defer ticker.Stop()

		progress := snapshot.Progress
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if progress >= tailRampMax {
					return
				}
				progress++

				updated, err := taskDao.UpdateTaskProgressIfHigher(ctx, snapshot.TaskID, progress)
				if err != nil {
					zlog.LogWithContext(ctx).Warn("推进收尾进度失败",
						zap.String("taskID", snapshot.TaskID),
						zap.Int32("progress", progress),
						zap.Error(err))
					return
				}
				// 没写进去说明主流程已经推进到更靠后的进度，后台不再干预
				if !updated {
					return
				}

				snapshot.Progress = progress
				SendProgressEvent(&snapshot)
			}
		}
	}()

	return &tailRamp{stop: sync.OnceFunc(func() {
		cancel()
		<-done
	})}
}

func (r *tailRamp) Stop() {
	r.stop()
}
