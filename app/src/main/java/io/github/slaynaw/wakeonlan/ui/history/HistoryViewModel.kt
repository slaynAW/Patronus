package io.github.slaynaw.wakeonlan.ui.history

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import io.github.slaynaw.wakeonlan.AgentJournalState
import io.github.slaynaw.wakeonlan.AppContainer
import io.github.slaynaw.wakeonlan.core.history.HistoryEvent
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.flow.stateIn

/** Évènement affiché, avec le nom du PC. */
data class HistoryItem(val event: HistoryEvent, val deviceName: String)

data class HistoryUiState(
    val loaded: Boolean = false,
    /** PC affiché ; `null` = tous. */
    val filter: String? = null,
    val events: List<HistoryItem> = emptyList(),
    /** PC configurés (identifiant, nom), pour le filtre. */
    val devices: List<Pair<String, String>> = emptyList(),
    /** Le PC affiché a un agent (journal complet, même application fermée). */
    val hasAgent: Boolean = false,
    val agentState: AgentJournalState? = null,
)

/** Historique des 30 derniers jours, d'un PC ou de tous. */
class HistoryViewModel(private val container: AppContainer, deviceId: String?) : ViewModel() {

    private val filter = MutableStateFlow(deviceId)

    val state: StateFlow<HistoryUiState> = combine(
        container.history.data,
        container.repository.config,
        container.historyTracker.agentJournal,
        filter,
    ) { data, config, journal, id ->
        val names = config.devices.associate { it.id to it.name }
        val selected = id?.takeIf { it in names }
        HistoryUiState(
            loaded = true,
            filter = selected,
            events = data.view(selected, System.currentTimeMillis()).mapNotNull { e ->
                names[e.device]?.let { HistoryItem(e, it) }
            },
            devices = config.devices.map { it.id to it.name },
            hasAgent = selected != null && config.devices.firstOrNull { it.id == selected }?.agent?.hasKey == true,
            agentState = selected?.let { journal[it] },
        )
    }.flowOn(Dispatchers.Default).stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), HistoryUiState(filter = deviceId))

    init {
        refresh()
    }

    fun select(id: String?) {
        filter.value = id
        refresh()
    }

    /** Relit le journal des agents concernés (au plus toutes les 20 s). */
    fun refresh() = container.historyTracker.refreshAgent(filter.value, force = true)
}
