import React from 'react'
import { Col, Container, Row } from 'react-bootstrap'

export default function Footer() {
  return (
    <div className='footer_wrapper'>
        <Container>
            <Row className='align-items-center'>
                <Col md="6" className='text-center text-md-left'>
                    <div>
                        <img src={require('../images/footer-logo.png')} className="img-fluid" alt="logo" />
                        <p>© 2021 - present, Sonatype Inc.</p>
                    </div>
                </Col>
                <Col md="6" className='text-center text-md-left'>
                    <ul className='list-inline'>
                        <li>CNCFbugbash@sonatype.com</li>
                        <li>Twitter : <a href="https://twitter.com/sonatypeDev">@sonatypeDev</a></li>
                    </ul>
                </Col>
            </Row>
        </Container>
    </div>
  )
}
