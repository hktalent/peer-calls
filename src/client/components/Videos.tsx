import React from 'react'
import { StreamWithURL, StreamsState, LocalStream } from '../reducers/streams'
import forEach from 'lodash/forEach'
import map from 'lodash/map'
import { ME } from '../constants'
import { getNickname } from '../nickname'
import Video from './Video'
import { Nicknames } from '../reducers/nicknames'
import { getStreamKey, WindowStates, WindowState } from '../reducers/windowStates'
import { MinimizeTogglePayload, StreamTypeCamera } from '../actions/StreamActions'

export interface VideosProps {
  nicknames: Nicknames
  play: () => void
  streams: StreamsState
  onMinimizeToggle: (payload: MinimizeTogglePayload) => void
  windowStates: WindowStates
}

interface StreamProps {
  key: string
  stream?: StreamWithURL
  peerId: string
  muted?: boolean
  localUser?: boolean
  mirrored?: boolean
  windowState: WindowState
}

export default class Videos extends React.PureComponent<VideosProps> {
  private gridRef = React.createRef<HTMLDivElement>()
  componentDidUpdate() {
    const videos = this.gridRef.current!
    .querySelectorAll('.video-container') as unknown as HTMLElement[]
    const size = videos.length
    const x = (1 / Math.ceil(Math.sqrt(size))) * 100

    videos.forEach(v => {
      v.style.flexBasis = x + '%'
    })
  }
  private getStreams() {
    const { windowStates, nicknames, streams } = this.props

    const minimized: StreamProps[] = []
    const maximized: StreamProps[] = []

    function addStreamProps(props: StreamProps) {
      if (props.windowState === 'minimized') {
        minimized.push(props)
      } else {
        maximized.push(props)
      }
    }

    function isLocalStream(s: StreamWithURL): s is LocalStream {
      return 'mirror' in s && 'type' in s
    }

    function addStreamsByUser(
      localUser: boolean,
      peerId: string,
      streams: Array<StreamWithURL | LocalStream>,
    ) {

      if (!streams.length) {
        const key = getStreamKey(peerId, undefined)
        const props: StreamProps = {
          key,
          peerId,
          localUser,
          windowState: windowStates[key],
        }
        addStreamProps(props)
        return
      }

      streams.forEach((stream) => {
        const key = getStreamKey(peerId, stream.streamId)
        const props: StreamProps = {
          key,
          stream: stream,
          peerId,
          mirrored: localUser && isLocalStream(stream) &&
            stream.type === StreamTypeCamera && stream.mirror,
          muted: localUser,
          localUser,
          windowState: windowStates[key],
        }
        addStreamProps(props)
      })
    }

    const localStreams = map(streams.localStreams, s => s!)
    addStreamsByUser(true, ME, localStreams)

    forEach(nicknames, (_, peerId) => {
      if (peerId != ME) {
        const s = map(
          streams.pubStreamsKeysByPeerId[peerId],
          (_, streamId) => streams.pubStreams[streamId],
        )
        .map(pubStream => streams.remoteStreams[pubStream.streamId])
        .filter(s => !!s)

        addStreamsByUser(false, peerId, s)
      }
    })

    return { minimized, maximized }
  }
  render() {
    const { minimized, maximized } = this.getStreams()

    const videosToolbar = (
      <div className="videos videos-toolbar" key="videos-toolbar">
        {minimized.map(props => (
          <Video
            {...props}
            key={props.key}
            onMinimizeToggle={this.props.onMinimizeToggle}
            play={this.props.play}
            nickname={getNickname(this.props.nicknames, props.peerId)}
          />
        ))}
      </div>
    )

    const videosGrid = (
      <div className="videos videos-grid" key="videos-grid" ref={this.gridRef}>
        {maximized.map(props => (
          <Video
            {...props}
            key={props.key}
            onMinimizeToggle={this.props.onMinimizeToggle}
            play={this.props.play}
            nickname={getNickname(this.props.nicknames, props.peerId)}
          />
        ))}
      </div>
    )

    return [videosToolbar, videosGrid]
  }
}
