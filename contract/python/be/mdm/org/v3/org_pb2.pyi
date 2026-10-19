from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class Aggregate(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    AGGREGATE_UNSPECIFIED: _ClassVar[Aggregate]
    AGGREGATE_LEGAL_ENTITY: _ClassVar[Aggregate]
    AGGREGATE_ORG_UNIT: _ClassVar[Aggregate]

class ReadState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    READ_STATE_UNSPECIFIED: _ClassVar[ReadState]
    READ_STATE_EXISTS: _ClassVar[ReadState]
    READ_STATE_TOMBSTONE: _ClassVar[ReadState]
    READ_STATE_PURGED: _ClassVar[ReadState]
AGGREGATE_UNSPECIFIED: Aggregate
AGGREGATE_LEGAL_ENTITY: Aggregate
AGGREGATE_ORG_UNIT: Aggregate
READ_STATE_UNSPECIFIED: ReadState
READ_STATE_EXISTS: ReadState
READ_STATE_TOMBSTONE: ReadState
READ_STATE_PURGED: ReadState

class Stamp(_message.Message):
    __slots__ = ("deployment", "instance", "epoch", "positions")
    DEPLOYMENT_FIELD_NUMBER: _ClassVar[int]
    INSTANCE_FIELD_NUMBER: _ClassVar[int]
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    POSITIONS_FIELD_NUMBER: _ClassVar[int]
    deployment: str
    instance: str
    epoch: str
    positions: _containers.RepeatedScalarFieldContainer[int]
    def __init__(self, deployment: _Optional[str] = ..., instance: _Optional[str] = ..., epoch: _Optional[str] = ..., positions: _Optional[_Iterable[int]] = ...) -> None: ...

class Record(_message.Message):
    __slots__ = ("id", "version", "data")
    ID_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    id: str
    version: int
    data: bytes
    def __init__(self, id: _Optional[str] = ..., version: _Optional[int] = ..., data: _Optional[bytes] = ...) -> None: ...

class SnapshotRequest(_message.Message):
    __slots__ = ("aggregate", "cursor", "limit")
    AGGREGATE_FIELD_NUMBER: _ClassVar[int]
    CURSOR_FIELD_NUMBER: _ClassVar[int]
    LIMIT_FIELD_NUMBER: _ClassVar[int]
    aggregate: Aggregate
    cursor: str
    limit: int
    def __init__(self, aggregate: _Optional[_Union[Aggregate, str]] = ..., cursor: _Optional[str] = ..., limit: _Optional[int] = ...) -> None: ...

class SnapshotResponse(_message.Message):
    __slots__ = ("records", "next_cursor", "stamp")
    RECORDS_FIELD_NUMBER: _ClassVar[int]
    NEXT_CURSOR_FIELD_NUMBER: _ClassVar[int]
    STAMP_FIELD_NUMBER: _ClassVar[int]
    records: _containers.RepeatedCompositeFieldContainer[Record]
    next_cursor: str
    stamp: Stamp
    def __init__(self, records: _Optional[_Iterable[_Union[Record, _Mapping]]] = ..., next_cursor: _Optional[str] = ..., stamp: _Optional[_Union[Stamp, _Mapping]] = ...) -> None: ...

class ReadRequest(_message.Message):
    __slots__ = ("aggregate", "ids")
    AGGREGATE_FIELD_NUMBER: _ClassVar[int]
    IDS_FIELD_NUMBER: _ClassVar[int]
    aggregate: Aggregate
    ids: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, aggregate: _Optional[_Union[Aggregate, str]] = ..., ids: _Optional[_Iterable[str]] = ...) -> None: ...

class ReadResult(_message.Message):
    __slots__ = ("id", "state", "version", "data")
    ID_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    DATA_FIELD_NUMBER: _ClassVar[int]
    id: str
    state: ReadState
    version: int
    data: bytes
    def __init__(self, id: _Optional[str] = ..., state: _Optional[_Union[ReadState, str]] = ..., version: _Optional[int] = ..., data: _Optional[bytes] = ...) -> None: ...

class ReadResponse(_message.Message):
    __slots__ = ("results", "stamp")
    RESULTS_FIELD_NUMBER: _ClassVar[int]
    STAMP_FIELD_NUMBER: _ClassVar[int]
    results: _containers.RepeatedCompositeFieldContainer[ReadResult]
    stamp: Stamp
    def __init__(self, results: _Optional[_Iterable[_Union[ReadResult, _Mapping]]] = ..., stamp: _Optional[_Union[Stamp, _Mapping]] = ...) -> None: ...

class PartitionPosition(_message.Message):
    __slots__ = ("partition", "epoch", "seq", "row_identity")
    PARTITION_FIELD_NUMBER: _ClassVar[int]
    EPOCH_FIELD_NUMBER: _ClassVar[int]
    SEQ_FIELD_NUMBER: _ClassVar[int]
    ROW_IDENTITY_FIELD_NUMBER: _ClassVar[int]
    partition: int
    epoch: str
    seq: int
    row_identity: str
    def __init__(self, partition: _Optional[int] = ..., epoch: _Optional[str] = ..., seq: _Optional[int] = ..., row_identity: _Optional[str] = ...) -> None: ...

class CatchUpRequest(_message.Message):
    __slots__ = ("nonce", "positions")
    NONCE_FIELD_NUMBER: _ClassVar[int]
    POSITIONS_FIELD_NUMBER: _ClassVar[int]
    nonce: str
    positions: _containers.RepeatedCompositeFieldContainer[PartitionPosition]
    def __init__(self, nonce: _Optional[str] = ..., positions: _Optional[_Iterable[_Union[PartitionPosition, _Mapping]]] = ...) -> None: ...

class CatchUpResponse(_message.Message):
    __slots__ = ("rows", "next_page", "stamp")
    ROWS_FIELD_NUMBER: _ClassVar[int]
    NEXT_PAGE_FIELD_NUMBER: _ClassVar[int]
    STAMP_FIELD_NUMBER: _ClassVar[int]
    rows: _containers.RepeatedScalarFieldContainer[bytes]
    next_page: str
    stamp: Stamp
    def __init__(self, rows: _Optional[_Iterable[bytes]] = ..., next_page: _Optional[str] = ..., stamp: _Optional[_Union[Stamp, _Mapping]] = ...) -> None: ...
