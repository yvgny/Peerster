package common

import "time"

const SharedFilesFolder = "_SharedFiles"
const DefaultBudget = 2
const MaxBudget = 32
const MatchThreshold = 2
const SearchRequestDuplicateTimer = time.Second / 2
const SearchRequestBudgetIncreasePeriod = time.Second
const RemoteSearchTimeout = time.Minute
const TxBroadcastHopLimit = 10
const BlockBroadcastHopLimit = 20
const HashMinLeadingZeroBitsLength = 18
const FirstBlockPublicationDelay = time.Second * 5