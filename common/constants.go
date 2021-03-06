package common

import "time"

const SharedFilesFolder = "_SharedFiles"
const CloudFilesUploadFolder = SharedFilesFolder
const DefaultBudget = 2
const MaxBudget = 32
const MatchThreshold = 2
const SearchRequestDuplicateTimer = time.Second / 2
const CloudRequestDuplicateTimer = SearchRequestDuplicateTimer
const SearchRequestBudgetIncreasePeriod = time.Second
const RemoteSearchTimeout = time.Minute
const TxBroadcastHopLimit = 10
const BlockBroadcastHopLimit = 20
const HashMinLeadingZeroBitsLength = 16
const FirstBlockPublicationDelay = time.Second * 5
const HiddenStorageFolder = ".peerster"
const CloudSearchTimeout = time.Second * 5
const ConfirmationThreshold = 15
const MaxPublicKeyPublishAttempt = 10
