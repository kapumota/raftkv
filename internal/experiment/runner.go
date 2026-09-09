package experiment

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type OperationResult struct {
	Sequence int           `json:"secuencia"`
	Client   int           `json:"cliente"`
	Key      string        `json:"clave"`
	Outcome  string        `json:"resultado"`
	Status   int           `json:"estado_http,omitempty"`
	Node     string        `json:"nodo,omitempty"`
	Error    string        `json:"error,omitempty"`
	Elapsed  time.Duration `json:"duracion_ns"`
}

type Event struct {
	Name string    `json:"nombre"`
	At   time.Time `json:"instante"`
	Node string    `json:"nodo,omitempty"`
	Term int       `json:"termino,omitempty"`
}

type ExperimentResult struct {
	Config     ExperimentConfig  `json:"configuracion"`
	Started    time.Time         `json:"inicio"`
	Finished   time.Time         `json:"fin"`
	Initial    map[string]string `json:"estado_inicial"`
	Events     []Event           `json:"eventos"`
	Operations []OperationResult `json:"operaciones"`
	Error      string            `json:"error,omitempty"`
}

func RunExperiment(config ExperimentConfig) (ExperimentResult, error) {
	return RunExperimentContext(context.Background(), config)
}

func RunExperimentContext(ctx context.Context, config ExperimentConfig) (ExperimentResult, error) {
	if err := config.Validate(); err != nil {
		return ExperimentResult{Config: config, Error: err.Error()}, err
	}
	return runExperiment(ctx, config, newDockerBackend(config))
}

func selectLeader(statuses []NodeStatus) (NodeStatus, error) {
	var leader NodeStatus
	count := 0
	for _, status := range statuses {
		if status.State == "lider" || status.State == "líder" {
			leader = status
			count++
		}
	}
	if count != 1 {
		return leader, fmt.Errorf("se esperaba un líder único, se observaron %d", count)
	}
	return leader, nil
}

func runExperiment(ctx context.Context, config ExperimentConfig, b backend) (result ExperimentResult, err error) {
	result.Config = config
	defer func() {
		result.Finished = time.Now().UTC()
		if err != nil {
			result.Error = err.Error()
		}
	}()
	if err = config.Validate(); err != nil {
		return
	}
	log.Printf("Iniciando experimento: %s", config.Name)
	ready, cancel := context.WithTimeout(ctx, 30*time.Second)
	var statuses []NodeStatus
	var leader NodeStatus
	for {
		statuses, err = b.Statuses(ready)
		if err == nil {
			leader, err = selectLeader(statuses)
		}
		allReady := len(statuses) == config.Nodes
		for _, status := range statuses {
			allReady = allReady && status.CommitIndex > 0
		}
		if err == nil && allReady {
			break
		}
		select {
		case <-ready.Done():
			cancel()
			return result, fmt.Errorf("el clúster no estuvo disponible: %w", ready.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
	cancel()
	result.Initial, err = b.Prepare(ctx)
	if err != nil {
		return
	}
	result.Started = time.Now().UTC()
	log.Printf("Líder detectado: %s", leader.ID)
	var hint atomic.Value
	hint.Store(leader.ID)
	workCtx, stopWork := context.WithCancel(ctx)
	workDone := make(chan []OperationResult, 1)
	started := result.Started
	go func() { workDone <- runLoad(workCtx, config, b, &hint, started) }()
	var target string
	dirty := false
	event := func(name string, term int) {
		result.Events = append(result.Events, Event{Name: name, At: time.Now().UTC(), Node: target, Term: term})
	}
	restore := func() error {
		log.Print("Restaurando conectividad y disponibilidad...")
		cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		if restoreErr := b.Restore(cleanup, config.Fault.Type, target); restoreErr != nil {
			event("restauracion_fallida", 0)
			return restoreErr
		}
		dirty = false
		event("restauracion_completada", 0)
		return nil
	}
	defer func() {
		if dirty {
			err = errors.Join(err, restore())
		}
		if err != nil {
			stopWork()
		}
		result.Operations = <-workDone
		stopWork()
	}()
	faultTimer := time.NewTimer(time.Duration(config.Fault.At) * time.Second)
	defer faultTimer.Stop()
	end := time.NewTimer(time.Duration(config.Duration) * time.Second)
	defer end.Stop()
	refresh := time.NewTicker(250 * time.Millisecond)
	defer refresh.Stop()
	var restoreAt <-chan time.Time
	var restoreTimer *time.Timer
	defer func() {
		if restoreTimer != nil {
			restoreTimer.Stop()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-faultTimer.C:
			if config.Fault.Type == "ninguna" {
				continue
			}
			statuses, err = b.Statuses(ctx)
			if err != nil {
				return
			}
			leader, err = selectLeader(statuses)
			if err != nil || len(statuses) != config.Nodes {
				return result, fmt.Errorf("no se pudo resolver el objetivo con todos los nodos disponibles: %v", err)
			}
			target = leader.ID
			targetTerm := leader.Term
			if config.Fault.Target == "seguidor" {
				sort.Slice(statuses, func(i, j int) bool { return statuses[i].ID < statuses[j].ID })
				target = ""
				for _, status := range statuses {
					if status.State == "seguidor" {
						target = status.ID
						targetTerm = status.Term
						break
					}
				}
				if target == "" {
					return result, fmt.Errorf("no se encontró un seguidor disponible")
				}
			}
			if time.Since(started)+time.Duration(config.Fault.Duration)*time.Second >= time.Duration(config.Duration)*time.Second {
				return result, fmt.Errorf("ya no queda tiempo para aplicar y restaurar la falla")
			}
			event("objetivo_seleccionado", targetTerm)
			dirty = true // También restaura si la aplicación falla después de modificar Docker.
			log.Printf("Aplicando %s sobre %s...", config.Fault.Type, target)
			err = b.Apply(ctx, config.Fault.Type, target)
			if err != nil {
				return
			}
			event("falla_aplicada", targetTerm)
			restoreTimer = time.NewTimer(time.Duration(config.Fault.Duration) * time.Second)
			restoreAt = restoreTimer.C
		case <-restoreAt:
			err = restore()
			if err != nil {
				return
			}
			restoreAt = nil
		case <-refresh.C:
			current, statusErr := b.Statuses(ctx)
			candidate, leaderErr := selectLeader(current)
			if statusErr == nil && leaderErr == nil {
				hint.Store(candidate.ID)
			} else {
				hint.Store("")
			}
		case <-end.C:
			if config.Fault.Type != "ninguna" && (target == "" || dirty) {
				return result, fmt.Errorf("la falla o su restauración excedieron la ventana prevista")
			}
			log.Print("Experimento finalizado.")
			return result, nil
		}
	}
}

func runLoad(ctx context.Context, config ExperimentConfig, b backend, hint *atomic.Value, start time.Time) []OperationResult {
	var mu sync.Mutex
	var results []OperationResult
	add := func(r OperationResult) { mu.Lock(); results = append(results, r); mu.Unlock() }
	jobs := make([]chan OperationResult, config.Clients)
	var workers sync.WaitGroup
	for i := range jobs {
		jobs[i] = make(chan OperationResult)
		workers.Add(1)
		go func(ch <-chan OperationResult) {
			defer workers.Done()
			for job := range ch {
				id := hint.Load().(string)
				job.Node = id
				if id == "" {
					job.Outcome = "omitida_sin_lider"
					add(job)
					continue
				}
				value := fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%d:%d", config.Seed, job.Sequence))))
				call, cancel := context.WithTimeout(ctx, 7*time.Second)
				began := time.Now()
				status, err := b.Write(call, id, job.Key, value)
				cancel()
				job.Elapsed, job.Status = time.Since(began), status
				if err != nil {
					job.Error = err.Error()
				}
				job.Outcome = "incierta"
				if err == nil && status == 200 {
					job.Outcome = "confirmada"
				}
				if err == nil && (status == 400 || status == 405) {
					job.Outcome = "rechazada"
				}
				add(job)
			}
		}(jobs[i])
	}
	interval := time.Second / time.Duration(config.Rate)
	for sequence := 0; sequence < config.Duration*config.Rate; sequence++ {
		due := start.Add(time.Duration(sequence) * interval)
		if delay := time.Until(due); delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				goto finish
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			break
		}
		job := OperationResult{Sequence: sequence, Client: sequence % config.Clients, Key: fmt.Sprintf("experimento-%s-%d-%d", config.Name, config.Seed, sequence)}
		job.Outcome = "omitida_saturacion"
		if time.Since(due) < interval {
			select {
			case jobs[job.Client] <- job:
				continue
			default:
			}
		}
		add(job)
	}
finish:
	for _, ch := range jobs {
		close(ch)
	}
	workers.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].Sequence < results[j].Sequence })
	return results
}
